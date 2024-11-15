package pipeline

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v3"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/metrics"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters"
	"github.com/algorand/conduit/conduit/plugins/importers"
	"github.com/algorand/conduit/conduit/plugins/processors"
	"github.com/algorand/conduit/conduit/telemetry"
)

// Pipeline is a struct that orchestrates the entire
// sequence of events, taking in importers, processors and
// exporters and generating the result
type Pipeline interface {
	Init() error
	Start()
	Stop()
	Error() error
	Wait()
	Status() (Status, error)
}

// Status is a struct that contains the current pipeline status.
type Status struct {
	Round uint64 `json:"round"`
}

type pipelineImpl struct {
	ctx      context.Context
	ccf      context.CancelCauseFunc
	wg       sync.WaitGroup
	cfg      *data.Config
	logger   *log.Logger
	profFile *os.File
	err      error
	errMu    sync.RWMutex

	initProvider *data.InitProvider

	importer         importers.Importer
	processors       []processors.Processor
	exporter         exporters.Exporter
	completeCallback []conduit.OnCompleteFunc

	pipelineMetadata state

	statusMu sync.Mutex
	status   Status
}

type pluginChannel chan data.BlockData

var (
	errImporterCause  = errors.New("importer cancelled")
	errProcessorCause = errors.New("processor cancelled")
	errExporterCause  = errors.New("exporter cancelled")
)

func (p *pipelineImpl) Error() error {
	p.errMu.RLock()
	defer p.errMu.RUnlock()
	return p.err
}

func (p *pipelineImpl) joinError(err error) {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	p.err = errors.Join(p.err, err)
}

func (p *pipelineImpl) cancelWithProblem(err error) {
	p.ccf(err)
	p.joinError(err)
}

func (p *pipelineImpl) registerLifecycleCallbacks() {
	if v, ok := p.importer.(conduit.Completed); ok {
		p.completeCallback = append(p.completeCallback, v.OnComplete)
	}
	for _, processor := range p.processors {
		if v, ok := processor.(conduit.Completed); ok {
			p.completeCallback = append(p.completeCallback, v.OnComplete)
		}
	}
	if v, ok := p.exporter.(conduit.Completed); ok {
		p.completeCallback = append(p.completeCallback, v.OnComplete)
	}
}

func (p *pipelineImpl) registerPluginMetricsCallbacks() {
	var collectors []prometheus.Collector
	if v, ok := p.importer.(conduit.PluginMetrics); ok {
		collectors = append(collectors, v.ProvideMetrics(p.cfg.Metrics.Prefix)...)
	}
	for _, processor := range p.processors {
		if v, ok := processor.(conduit.PluginMetrics); ok {
			collectors = append(collectors, v.ProvideMetrics(p.cfg.Metrics.Prefix)...)
		}
	}
	if v, ok := p.exporter.(conduit.PluginMetrics); ok {
		collectors = append(collectors, v.ProvideMetrics(p.cfg.Metrics.Prefix)...)
	}
	for _, c := range collectors {
		_ = prometheus.Register(c)
	}
}

// makeConfig creates a plugin config from a name and config pair.
// It also creates a logger for the plugin and configures it using the pipeline's log settings.
func (p *pipelineImpl) makeConfig(cfg data.NameConfigPair, pluginType plugins.PluginType) (*log.Logger, plugins.PluginConfig, error) {
	configs, err := yaml.Marshal(cfg.Config)
	if err != nil {
		return nil, plugins.PluginConfig{}, fmt.Errorf("makeConfig(): could not serialize config: %w", err)
	}

	lgr := log.New()
	lgr.SetOutput(p.logger.Out)
	lgr.SetLevel(p.logger.Level)
	lgr.SetFormatter(makePluginLogFormatter(string(pluginType), cfg.Name))

	var config plugins.PluginConfig
	config.Config = string(configs)
	if p.cfg != nil && p.cfg.ConduitArgs != nil {
		config.DataDir = path.Join(p.cfg.ConduitArgs.ConduitDataDir, fmt.Sprintf("%s_%s", pluginType, cfg.Name))
		err = os.MkdirAll(config.DataDir, os.ModePerm)
		if err != nil {
			return nil, plugins.PluginConfig{}, fmt.Errorf("makeConfig: unable to create plugin data directory: %w", err)
		}
	}

	return lgr, config, nil
}

// pluginRoundOverride looks at the round override argument, and attempts to query
// each plugin for a requested round override. If there is no override, 0 is
// returned, if there is one or more round override which are in agreement,
// that round is returned, if there are multiple different round overrides
// an error is returned.
func (p *pipelineImpl) pluginRoundOverride() (uint64, error) {
	type overrideFunc func(config plugins.PluginConfig) (uint64, error)
	type overridePart struct {
		RoundRequest overrideFunc
		cfg          data.NameConfigPair
		t            plugins.PluginType
	}
	var parts []overridePart

	if v, ok := p.importer.(conduit.RoundRequestor); ok {
		parts = append(parts, overridePart{
			RoundRequest: v.RoundRequest,
			cfg:          p.cfg.Importer,
			t:            plugins.Importer,
		})
	}
	for idx, processor := range p.processors {
		if v, ok := processor.(conduit.RoundRequestor); ok {
			parts = append(parts, overridePart{
				RoundRequest: v.RoundRequest,
				cfg:          p.cfg.Processors[idx],
				t:            plugins.Processor,
			})
		}
	}
	if v, ok := p.exporter.(conduit.RoundRequestor); ok {
		parts = append(parts, overridePart{
			RoundRequest: v.RoundRequest,
			cfg:          p.cfg.Exporter,
			t:            plugins.Exporter,
		})
	}

	// Call the override functions.
	var pluginOverride uint64
	var pluginOverrideName string // cache this in case of error.
	for _, part := range parts {
		_, config, err := p.makeConfig(part.cfg, part.t)
		if err != nil {
			return 0, err
		}
		rnd, err := part.RoundRequest(config)
		if err != nil {
			return 0, err
		}
		if pluginOverride != 0 && rnd != 0 && rnd != pluginOverride {
			return 0, makeErrOverrideConflict(pluginOverrideName, pluginOverride, part.cfg.Name, rnd)
		}
		if rnd != 0 {
			pluginOverride = rnd
			pluginOverrideName = part.cfg.Name
		}
	}

	// Check command line arg
	if pluginOverride != 0 && p.cfg.ConduitArgs.NextRoundOverride != 0 && p.cfg.ConduitArgs.NextRoundOverride != pluginOverride {
		return 0, makeErrOverrideConflict(pluginOverrideName, pluginOverride, "command line", p.cfg.ConduitArgs.NextRoundOverride)
	}
	if p.cfg.ConduitArgs.NextRoundOverride != 0 {
		pluginOverride = p.cfg.ConduitArgs.NextRoundOverride
	}

	return pluginOverride, nil
}

// initializeTelemetry initializes telemetry and reads or sets the GUID in the metadata.
func (p *pipelineImpl) initializeTelemetry() (telemetry.Client, error) {
	telemetryConfig := telemetry.MakeTelemetryConfig(p.cfg.Telemetry.URI, p.cfg.Telemetry.Index, p.cfg.Telemetry.UserName, p.cfg.Telemetry.Password)
	telemetryClient, err := telemetry.MakeOpenSearchClient(telemetryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}
	p.logger.Infof("Telemetry initialized with URI: %s", telemetryConfig.URI)

	// If GUID is not in metadata, save it. Otherwise, use the GUID from metadata.
	if p.pipelineMetadata.TelemetryID == "" {
		p.pipelineMetadata.TelemetryID = telemetryClient.TelemetryConfig.GUID
	} else {
		telemetryClient.TelemetryConfig.GUID = p.pipelineMetadata.TelemetryID
	}

	return telemetryClient, nil
}

// Init prepares the pipeline for processing block data
func (p *pipelineImpl) Init() error {
	p.logger.Infof("Starting Pipeline Initialization")

	if p.cfg.Metrics.Prefix == "" {
		p.cfg.Metrics.Prefix = data.DefaultMetricsPrefix
	}
	metrics.RegisterPrometheusMetrics(p.cfg.Metrics.Prefix)

	if p.cfg.CPUProfile != "" {
		p.logger.Infof("Creating CPU Profile file at %s", p.cfg.CPUProfile)
		var err error
		profFile, err := os.Create(p.cfg.CPUProfile)
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): unable to create profile: %w", err)
		}
		p.profFile = profFile
		err = pprof.StartCPUProfile(profFile)
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): unable to start pprof: %w", err)
		}
	}

	if p.cfg.PIDFilePath != "" {
		err := createPidFile(p.logger, p.cfg.PIDFilePath)
		if err != nil {
			return err
		}
	}

	// Read metadata file if it exists. We have to do this here in order to get
	// the round from a previous run if it exists.
	p.pipelineMetadata = state{}
	populated, err := isFilePopulated(metadataPath(p.cfg.ConduitArgs.ConduitDataDir))
	if err != nil {
		return fmt.Errorf("Pipeline.Init(): could not stat metadata: %w", err)
	}
	if populated {
		p.pipelineMetadata, err = readBlockMetadata(p.cfg.ConduitArgs.ConduitDataDir)
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): could not read metadata: %w", err)
		}
	}

	// Check for round override
	p.logger.Infof("Checking for round override")
	pluginRoundOverride, err := p.pluginRoundOverride()
	if err != nil {
		return fmt.Errorf("Pipeline.Init(): error resolving plugin round override: %w", err)
	}
	if pluginRoundOverride > 0 && pluginRoundOverride != p.pipelineMetadata.NextRound {
		p.logger.Infof("Overriding default next round from %d to %d.", p.pipelineMetadata.NextRound, pluginRoundOverride)
		p.pipelineMetadata.NextRound = pluginRoundOverride
	} else {
		p.logger.Infof("Initializing to pipeline round %d.", p.pipelineMetadata.NextRound)
	}

	// InitProvider
	round := sdk.Round(p.pipelineMetadata.NextRound)

	// Initialize Telemetry
	var telemetryClient telemetry.Client
	if p.cfg.Telemetry.Enabled {
		// If telemetry cannot be initialized, log a warning and continue
		// pipeline initialization.
		var telemetryErr error
		telemetryClient, telemetryErr = p.initializeTelemetry()
		if telemetryErr != nil {
			p.logger.Warnf("Telemetry initialization failed, continuing without telemetry: %s", telemetryErr)
		} else {
			// Try sending a startup event. If it fails, log a warning and continue
			event := telemetryClient.MakeTelemetryStartupEvent()
			if telemetryErr = telemetryClient.SendEvent(event); telemetryErr != nil {
				p.logger.Warnf("failed to send telemetry event: %s", telemetryErr)
			}
		}
	}

	// Initial genesis object is nil and gets updated after importer.Init
	var initProvider data.InitProvider = conduit.MakePipelineInitProvider(&round, nil, telemetryClient)
	p.initProvider = &initProvider

	// Initialize Importer
	{
		importerLogger, pluginConfig, err := p.makeConfig(p.cfg.Importer, plugins.Importer)
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): could not make %s config: %w", p.cfg.Importer.Name, err)
		}
		err = p.importer.Init(p.ctx, *p.initProvider, pluginConfig, importerLogger)
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): could not initialize importer (%s): %w", p.cfg.Importer.Name, err)
		}
		genesis, err := p.importer.GetGenesis()
		if err != nil {
			return fmt.Errorf("Pipeline.GetGenesis(): could not obtain Genesis from the importer (%s): %w", p.cfg.Importer.Name, err)
		}
		(*p.initProvider).SetGenesis(genesis)

		// write pipeline metadata
		gh := genesis.Hash()
		ghbase64 := base64.StdEncoding.EncodeToString(gh[:])
		if p.pipelineMetadata.GenesisHash != "" && p.pipelineMetadata.GenesisHash != ghbase64 {
			return fmt.Errorf("Pipeline.Init(): genesis hash in metadata does not match expected value: actual %s, expected %s", gh, p.pipelineMetadata.GenesisHash)
		}
		p.pipelineMetadata.GenesisHash = ghbase64
		p.pipelineMetadata.Network = genesis.Network
		err = p.pipelineMetadata.encodeToFile(p.cfg.ConduitArgs.ConduitDataDir)
		if err != nil {
			return fmt.Errorf("Pipeline.Init() failed to write metadata to file: %w", err)
		}

		p.logger.Infof("Initialized Importer: %s", p.cfg.Importer.Name)
	}

	// Initialize Processors
	for idx, processor := range p.processors {
		ncPair := p.cfg.Processors[idx]
		logger, config, err := p.makeConfig(ncPair, plugins.Processor)
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): could not initialize processor (%s): %w", ncPair, err)
		}
		err = processor.Init(p.ctx, *p.initProvider, config, logger)
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): could not initialize processor (%s): %w", ncPair.Name, err)
		}
		p.logger.Infof("Initialized Processor: %s", ncPair.Name)
	}

	// Initialize Exporter
	{
		logger, config, err := p.makeConfig(p.cfg.Exporter, plugins.Exporter)
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): could not initialize processor (%s): %w", p.cfg.Exporter.Name, err)
		}
		err = p.exporter.Init(p.ctx, *p.initProvider, config, logger)
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): could not initialize Exporter (%s): %w", p.cfg.Exporter.Name, err)
		}
		p.logger.Infof("Initialized Exporter: %s", p.cfg.Exporter.Name)
	}

	// Register callbacks.
	p.registerLifecycleCallbacks()

	// start metrics server
	if p.cfg.Metrics.Mode == "ON" {
		p.registerPluginMetricsCallbacks()
		go p.startMetricsServer()
	}

	p.statusMu.Lock()
	defer p.statusMu.Unlock()
	p.status.Round = p.pipelineMetadata.NextRound

	return err
}

func (p *pipelineImpl) Stop() {
	p.ccf(nil)
	p.wg.Wait()

	if p.profFile != nil {
		if err := p.profFile.Close(); err != nil {
			p.logger.WithError(err).Errorf("%s: could not close CPUProf file", p.profFile.Name())
		}
		pprof.StopCPUProfile()
	}

	if p.cfg.PIDFilePath != "" {
		if err := os.Remove(p.cfg.PIDFilePath); err != nil {
			p.logger.WithError(err).Errorf("%s: could not remove pid file", p.cfg.PIDFilePath)
		}
	}

	if err := p.importer.Close(); err != nil {
		// Log and continue on closing the rest of the pipeline
		p.logger.Errorf("Pipeline.Stop(): Importer (%s) error on close: %v", p.importer.Metadata().Name, err)
	}

	for _, processor := range p.processors {
		if err := processor.Close(); err != nil {
			// Log and continue on closing the rest of the pipeline
			p.logger.Errorf("Pipeline.Stop(): Processor (%s) error on close: %v", processor.Metadata().Name, err)
		}
	}

	if err := p.exporter.Close(); err != nil {
		p.logger.Errorf("Pipeline.Stop(): Exporter (%s) error on close: %v", p.exporter.Metadata().Name, err)
	}
}

func numInnerTxn(txn sdk.SignedTxnWithAD) int {
	result := 0
	for _, itxn := range txn.ApplyData.EvalDelta.InnerTxns {
		result += 1 + numInnerTxn(itxn)
	}
	return result
}

func addMetrics(block data.BlockData, importTime time.Duration) {
	metrics.BlockImportTimeSeconds.Observe(importTime.Seconds())
	metrics.ImportedRoundGauge.Set(float64(block.Round()))
	txnCountByType := make(map[string]int)
	innerTxn := 0
	for _, txn := range block.Payset {
		txnCountByType[string(txn.Txn.Type)]++
		innerTxn += numInnerTxn(txn.SignedTxnWithAD)
	}
	if innerTxn != 0 {
		txnCountByType["inner"] = innerTxn
	}
	for k, v := range txnCountByType {
		metrics.ImportedTxns.WithLabelValues(k).Set(float64(v))
	}
	metrics.ImportedTxnsPerBlock.Observe(float64(len(block.Payset)) + float64(innerTxn))
}

func (p *pipelineImpl) importerHandler(importer importers.Importer, roundChan <-chan uint64, blkOutChan pluginChannel) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		lastRnd := uint64(0)
		totalSelectWait := time.Duration(0)
		totalFeedWait := time.Duration(0)
		defer func() {
			p.logger.Debugf("importer handler exiting. lastRnd=%d totalSelectWait=%dms; totalFeedWait=%dms", lastRnd, totalSelectWait.Milliseconds(), totalFeedWait.Milliseconds())
		}()

		for {
			startRound := time.Now()
			select {
			case <-p.ctx.Done():
				p.logger.Infof("importer handler exiting. lastRnd=%d", lastRnd)
				return
			case rnd := <-roundChan:
				lastRnd = rnd
				waitTime := time.Since(startRound)
				totalSelectWait += waitTime
				p.logger.Tracef("importer handler waited %dms to receive round %d", waitTime.Milliseconds(), rnd)

				blkData, importTime, lastError := retries(importer.GetBlock, rnd, p, importer.Metadata().Name)
				if lastError != nil {
					p.cancelWithProblem(fmt.Errorf("importer %s handler (%w): failed to import round %d after %dms: %w", importer.Metadata().Name, errImporterCause, rnd, importTime.Milliseconds(), lastError))
					return
				}
				metrics.ImporterTimeSeconds.Observe(importTime.Seconds())
				p.logger.Tracef("importer handler imported round %d in %dms", rnd, importTime.Milliseconds())

				// TODO: Verify that the block was built with a known protocol version.

				importFinish := time.Now()

				// check context in case out channel is full.
				select {
				case <-p.ctx.Done():
					return
				case blkOutChan <- blkData:
					waitTime := time.Since(importFinish)
					totalFeedWait += waitTime
					p.logger.Tracef("imported round %d into blkOutChan after waiting %dms on channel", rnd, waitTime.Milliseconds())
				}

			}
		}
	}()
}

func (p *pipelineImpl) processorHandler(idx int, proc processors.Processor, blkInChan pluginChannel, blkOutChan pluginChannel) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		var lastRnd uint64
		totalSelectWait := time.Duration(0)
		totalFeedWait := time.Duration(0)
		defer func() {
			p.logger.Debugf("processor[%d] %s handler exiting. lastRnd=%d totalSelectWait=%dms, totalFeedWait=%dms", idx, proc.Metadata().Name, lastRnd, totalSelectWait.Milliseconds(), totalFeedWait.Milliseconds())
		}()

		for {
			selectStart := time.Now()
			select {
			case <-p.ctx.Done():
				p.logger.Infof("processor[%d] %s handler exiting lastRnd=%d", idx, proc.Metadata().Name, lastRnd)
				return
			case blk := <-blkInChan:
				waitTime := time.Since(selectStart)
				totalSelectWait += waitTime
				p.logger.Tracef("processor[%d] %s handler received block data for round %d after wait of %s", idx, proc.Metadata().Name, lastRnd, waitTime)
				lastRnd = blk.Round()

				var procTime time.Duration
				var lastError error
				blk, procTime, lastError = retries(proc.Process, blk, p, proc.Metadata().Name)
				if lastError != nil {
					p.cancelWithProblem(fmt.Errorf("processor[%d] %s handler (%w): failed to process round %d after %dms: %w", idx, proc.Metadata().Name, errProcessorCause, lastRnd, procTime.Milliseconds(), lastError))
					return
				}
				metrics.ProcessorTimeSeconds.WithLabelValues(proc.Metadata().Name).Observe(procTime.Seconds())

				selectStart := time.Now()
				// check context in case out channel is full.
				select {
				case <-p.ctx.Done():
					return
				case blkOutChan <- blk:
					waitTime := time.Since(selectStart)
					totalFeedWait += waitTime
					p.logger.Tracef("processor[%d] %s for round %d into blkOutChan after waiting %dms", idx, proc.Metadata().Name, lastRnd, totalFeedWait.Milliseconds())
				}
			}
		}
	}()
}

// exporterHandler handles the exporter's Receive method, updating the metadata's NextRound,
// saving the metadata, and invoking the activated plugin callbacks. If the context's cancellation is received
// before the exporter's Receive method has succeeded, cancel immediately.
// However, after the exporter's Receive() method has succeeded we try every component at least once
// but will abort if a failure occurs after a cancellation is received.
func (p *pipelineImpl) exporterHandler(exporter exporters.Exporter, blkChan pluginChannel) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		var lastError error
		lastRound := uint64(0)
		totalSelectWait := time.Duration(0)
		totalExportWork := time.Duration(0)
		eName := exporter.Metadata().Name

		defer func() {
			p.logger.Debugf("exporter %s handler exiting. lastRnd=%d totalSelectWait=%dms, totalExportWork=%dms", eName, lastRound, totalSelectWait.Milliseconds(), totalExportWork.Milliseconds())
			if lastError != nil {
				err := fmt.Errorf("exporter %s handler (%w) after round %d: %w", eName, errExporterCause, lastRound, lastError)
				p.cancelWithProblem(err)
				p.logger.Error(err)
			}
		}()

		for {
			p.logger.Tracef("exporter handler waiting for block data. round=%d", lastRound)
			selectStart := time.Now()
			select {
			case <-p.ctx.Done():
				p.logger.Infof("exporter handler exiting lastRnd=%d", lastRound)
				return
			case blk := <-blkChan:
				waitTime := time.Since(selectStart)
				totalSelectWait += waitTime
				p.logger.Tracef("exporter handler received block data for round %d after wait of %s", lastRound, waitTime)
				lastRound = blk.Round()

				if p.pipelineMetadata.NextRound != lastRound {
					lastError = fmt.Errorf("aborting after out of order block data. %d != %d", p.pipelineMetadata.NextRound, lastRound)
					return
				}

				var exportTime time.Duration
				exportTime, lastError = retriesNoOutput(exporter.Receive, blk, p, eName)
				if lastError != nil {
					lastError = fmt.Errorf("aborting after failing to export round %d: %w", lastRound, lastError)
					return
				}
				metrics.ExporterTimeSeconds.Observe(exportTime.Seconds())
				// Ignore round 0 (which is empty).
				if lastRound > 0 {
					// Previously we reported time starting after block fetching is complete
					// through the end of the export operation. Now that each plugin is running
					// in its own goroutine, report only the time of the export.
					addMetrics(blk, exportTime)
				}

				// Increment Round, update metadata
				nextRound := lastRound + 1
				p.pipelineMetadata.NextRound = nextRound
				p.statusMu.Lock()
				p.status.Round = nextRound
				p.statusMu.Unlock()
				lastError = p.pipelineMetadata.encodeToFile(p.cfg.ConduitArgs.ConduitDataDir)
				if lastError != nil {
					lastError = fmt.Errorf("aborting after updating NextRound=%d BUT failing to save metadata: %w", nextRound, lastError)
					return
				}
				p.logger.Tracef("exporter %s incremented pipeline metadata NextRound to %d", eName, nextRound)

				for i, cb := range p.completeCallback {
					p.logger.Tracef("exporter %s @ round=%d NextRound=%d executing callback %d", eName, lastRound, nextRound, i)
					callbackErr := cb(blk)
					if callbackErr != nil {
						p.logger.Errorf(
							"exporter %s # round %d failed callback #%d but CONTINUING to NextRound=%d: %v",
							eName, lastRound, i, nextRound, callbackErr,
						)
					}
				}

				lastError = nil
				// WARNING: removing/re-log-levelling the following will BREAK:
				// - the E2E test (Search for "Pipeline round" in subslurp.py)
				// - the internal tools logstats collector (See func ConduitCollector in logstats.go of internal-tools repo)
				p.logger.Info(logstatsE2Elog(lastRound, len(blk.Payset), exportTime))
			}
		}
	}()
}

func logstatsE2Elog(lastRound uint64, topLevelTxnCount int, exportTime time.Duration) string {
	return fmt.Sprintf(
		"FINISHED Pipeline round r=%d (%d txn) exported in %s",
		lastRound,
		topLevelTxnCount,
		exportTime,
	)
}

// Start pushes block data through the pipeline
func (p *pipelineImpl) Start() {
	p.logger.Trace("Pipeline.Start()")

	// Setup channels
	roundChan := make(chan uint64)
	processorBlkInChan := make(pluginChannel)
	p.importerHandler(p.importer, roundChan, processorBlkInChan)

	var processorBlkOutChan pluginChannel
	for i, proc := range p.processors {
		processorBlkOutChan = make(pluginChannel)
		p.processorHandler(i, proc, processorBlkInChan, processorBlkOutChan)
		processorBlkInChan = processorBlkOutChan
	}

	p.exporterHandler(p.exporter, processorBlkInChan)

	p.wg.Add(1)
	// Main loop
	go func(startRound uint64) {
		defer p.wg.Done()
		rnd := startRound
		totalFeedWait := time.Duration(0)
		defer p.logger.Debugf("round channel feed exiting. lastRnd=%d totalFeedWait=%dms", rnd, totalFeedWait.Milliseconds())
		for {
			selectStart := time.Now()
			p.logger.Infof("Pipeline round kickoff: %v", rnd)
			p.logger.Tracef("pushing round %d into roundChan", rnd)
			select {
			case <-p.ctx.Done():
				p.logger.Infof("round channel feed exiting. lastRnd=%d", rnd)
				return
			case roundChan <- rnd:
				waitTime := time.Since(selectStart)
				totalFeedWait += waitTime
				rnd++
			}
		}
	}(p.pipelineMetadata.NextRound)
}

func (p *pipelineImpl) Wait() {
	p.wg.Wait()
}

func (p *pipelineImpl) Status() (Status, error) {
	p.statusMu.Lock()
	ret := p.status
	p.statusMu.Unlock()
	return ret, nil
}

// start a http server serving /metrics
func (p *pipelineImpl) startMetricsServer() {
	http.Handle("/metrics", promhttp.Handler())
	_ = http.ListenAndServe(p.cfg.Metrics.Addr, nil)
	p.logger.Infof("conduit metrics serving on %s", p.cfg.Metrics.Addr)
}

// MakePipeline creates a Pipeline
func MakePipeline(ctx context.Context, cfg *data.Config, logger *log.Logger) (Pipeline, error) {

	if cfg == nil {
		return nil, fmt.Errorf("MakePipeline(): pipeline config was empty")
	}

	if err := cfg.Valid(); err != nil {
		return nil, fmt.Errorf("MakePipeline(): %w", err)
	}

	if logger == nil {
		return nil, fmt.Errorf("MakePipeline(): logger was empty")
	}

	cancelContext, cancelCauseFunc := context.WithCancelCause(ctx)

	pipeline := &pipelineImpl{
		ctx:          cancelContext,
		ccf:          cancelCauseFunc,
		cfg:          cfg,
		logger:       logger,
		initProvider: nil,
		importer:     nil,
		processors:   []processors.Processor{},
		exporter:     nil,
	}

	importerName := cfg.Importer.Name

	importerConstructor, err := importers.ImporterConstructorByName(importerName)
	if err != nil {
		return nil, fmt.Errorf("MakePipeline(): could not build importer '%s': %w", importerName, err)
	}

	pipeline.importer = importerConstructor.New()
	logger.Infof("Found Importer: %s", importerName)

	// ---

	for _, processorConfig := range cfg.Processors {
		processorName := processorConfig.Name

		processorConstructor, err := processors.ProcessorConstructorByName(processorName)
		if err != nil {
			return nil, fmt.Errorf("MakePipeline(): could not build processor '%s': %w", processorName, err)
		}

		pipeline.processors = append(pipeline.processors, processorConstructor.New())
		logger.Infof("Found Processor: %s", processorName)
	}

	// ---

	exporterName := cfg.Exporter.Name

	exporterConstructor, err := exporters.ExporterConstructorByName(exporterName)
	if err != nil {
		return nil, fmt.Errorf("MakePipeline(): could not build exporter '%s': %w", exporterName, err)
	}

	pipeline.exporter = exporterConstructor.New()
	logger.Infof("Found Exporter: %s", exporterName)

	return pipeline, nil
}

// createPidFile creates the pid file at the specified location. Copied from Indexer util.CreateIndexerPidFile.
func createPidFile(logger *log.Logger, pidFilePath string) error {
	var err error
	logger.Infof("Creating PID file at: %s", pidFilePath)
	fout, err := os.Create(pidFilePath)
	if err != nil {
		err = fmt.Errorf("%s: could not create pid file, %v", pidFilePath, err)
		logger.Error(err)
		return err
	}

	if _, err = fmt.Fprintf(fout, "%d", os.Getpid()); err != nil {
		err = fmt.Errorf("%s: could not write pid file, %v", pidFilePath, err)
		logger.Error(err)
		return err
	}

	err = fout.Close()
	if err != nil {
		err = fmt.Errorf("%s: could not close pid file, %v", pidFilePath, err)
		logger.Error(err)
		return err
	}
	return err
}
