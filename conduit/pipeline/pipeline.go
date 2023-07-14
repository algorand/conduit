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
	"gopkg.in/yaml.v3"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/metrics"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters"
	"github.com/algorand/conduit/conduit/plugins/importers"
	"github.com/algorand/conduit/conduit/plugins/processors"
	"github.com/algorand/conduit/conduit/telemetry"
	"github.com/algorand/conduit/conduit/types"
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
}

type timedBlockData struct {
	start time.Time
	block *data.BlockData
}

type pipelineRoundError struct {
	err   error
	round uint64
}

type RoundBroadcaster struct {
	types.Broadcaster[uint64]
}

func NewRoundBroadcaster(logger *log.Logger) *RoundBroadcaster {
	return &RoundBroadcaster{
		Broadcaster: *types.NewBroadcaster[uint64](logger),
	}
}

func (e *pipelineRoundError) Error() string {
	return fmt.Sprintf("pipeline round %d: %v", e.round, e.err)
}

func (e *pipelineRoundError) Unwrap() error {
	return e.err
}

func newPipelineRoundError(err error, round uint64) *pipelineRoundError {
	return &pipelineRoundError{err: err, round: round}
}

type pipelineImpl struct {
	ctx      context.Context
	cf       context.CancelFunc
	wg       sync.WaitGroup
	cfg      *data.Config
	logger   *log.Logger
	profFile *os.File
	err      error
	errMu    sync.RWMutex

	initProvider *data.InitProvider

	importer         importers.Importer
	chanBuffSize     int
	processors       []processors.Processor
	exporter         exporters.Exporter
	completeCallback []conduit.OnCompleteFunc

	pipelineMetadata state
}

func (p *pipelineImpl) Error() error {
	p.errMu.RLock()
	defer p.errMu.RUnlock()
	return p.err
}

func (p *pipelineImpl) setError(err error) {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	p.err = err
}

func (p *pipelineImpl) joinError(err error) {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	p.err = errors.Join(p.err, err)
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
	if pluginRoundOverride > 0 && pluginRoundOverride != p.pipelineMetadata.NextRoundDEPRECATED {
		p.logger.Infof("Overriding default next round from %d to %d.", p.pipelineMetadata.NextRoundDEPRECATED, pluginRoundOverride)
		p.pipelineMetadata.NextRoundDEPRECATED = pluginRoundOverride
	} else {
		p.logger.Infof("Initializing to pipeline round %d.", p.pipelineMetadata.NextRoundDEPRECATED)
	}

	// InitProvider
	round := sdk.Round(p.pipelineMetadata.NextRoundDEPRECATED)

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

	return err
}

func (p *pipelineImpl) Stop() {
	p.cf()
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
	// TODO: close importChannel[0]

	for _, processor := range p.processors {
		if err := processor.Close(); err != nil {
			// Log and continue on closing the rest of the pipeline
			p.logger.Errorf("Pipeline.Stop(): Processor (%s) error on close: %v", processor.Metadata().Name, err)
		}
		// TODO: close procChannels[i]
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

// oneRound attempts to complete one round of the pipeline given a particular
// partial state of type roundState.
// It should only be called after the importer.GetBlock() pre-condition is met.
// Before attempting export, it blocks until the exorter.Receive() pre-condition is met.
// The state machine works as follows:
// - beforePlugins: regardless of plugin call history, try importer.Get(), processors.Process(), exporter.Export()
// - roundStateNeedsCallbacks: plugins have succeeded and regardless of callback history, try calling all of the completeCallback's
// - roundStateNeedsSaveMetadata: pipeline's NextRound is updated, try importer.SaveMetadata()
// - completed: this is the final state
func (p *pipelineImpl) oneRound(round uint64, state roundState, prevBlk *data.BlockData, ctx context.Context, waitSig, doneSig roundComplete) (roundState, *data.BlockData, error) {
	p.logger.Debugf("Pipeline.oneRound(): round %d, state %d", round, state)

	if state >= completed {
		return state, prevBlk, fmt.Errorf("Pipeline.oneRound(): received state >= roundStateComplete @ round=%d state=%d", round, state)
	}

	var blk data.BlockData
	if prevBlk != nil {
		if state <= beforePlugins {
			return state, prevBlk, fmt.Errorf("Pipeline.oneRound(): received non-nil prevBlk @ round=%d state=%d", round, state)
		}
		blk = *prevBlk
	}

	if state <= beforePlugins {
		p.logger.Debugf("Pipeline.oneRound(): round %d, state %d: beforePlugins", round, state)
		var err error
		select {
		case <-p.ctx.Done():
			p.logger.Debugf("Pipeline.oneRound(): round %d, state %d: <-p.ctx.Done() before importer", round, state)
			return state, &blk, p.ctx.Err()
		default:
			if blk, err = p.importer.GetBlock(round); err != nil {
				return state, &blk, err
			}
		}
		for _, proc := range p.processors {
			select {
			case <-p.ctx.Done():
				p.logger.Debugf("Pipeline.oneRound(): round %d, state %d: <-p.ctx.Done() before processor %s", round, state, proc.Metadata().Name)
				return state, &blk, p.ctx.Err()
			default:
				if blk, err = proc.Process(blk); err != nil {
					return state, &blk, err
				}
			}
		}
		select {
		case <-p.ctx.Done():
			p.logger.Debugf("Pipeline.oneRound(): round %d, state %d: <-p.ctx.Done() before exporter", round, state)
			return state, &blk, p.ctx.Err()
		case <-waitSig:
			// the Receive() pre-condition has been met
			if err = p.exporter.Receive(blk); err != nil {
				return state, &blk, err
			}
			state = beforeCallbacks
		}
	}

	if state <= beforeCallbacks {
		p.logger.Debugf("Pipeline.oneRound(): round %d, state %d: beforeCallbacks", round, state)
		for _, cb := range p.completeCallback {
			// TODO: I'm assuming that the callbacks are idempotent.
			// Is this assumption correct?
			err := cb(blk)
			if err != nil {
				return state, &blk, err
			}
		}
		p.pipelineMetadata.NextRound.Store(round + 1)
		state = beforeMetadataSave
	}

	if state <= beforeMetadataSave {
		p.logger.Debugf("Pipeline.oneRound(%d): attempt save pipeline metadata", round)
		// return state, &blk, fmt.Errorf("ARTIFICIAL ERROR round %d", round)
		err := p.pipelineMetadata.encodeToFile(p.cfg.ConduitArgs.ConduitDataDir)
		if err != nil {
			p.logger.WithError(err).Errorf("Pipeline.oneRound(): could not save pipeline metadata")
			return state, &blk, err
		}
		state = completed
	}

	// only a successful run can close the output roundSignal
	close(doneSig)

	// if we got here, we've succeeded and there's no need to provide the block to the caller
	// for a retry so let it be garbage collected ASAP
	return state, nil, nil
}

// oneRoundWithRetries is a goroutine for retrying oneRound() up to p.cfg.RetryCount times
func (p *pipelineImpl) oneRoundWithRetries(round uint64, exportSig roundComplete, ctx context.Context, cf context.CancelCauseFunc) roundComplete {
	p.logger.Debugf("Pipeline.oneRoundWithRetries(%d)", round)

	roundSig := make(roundComplete)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		var state roundState
		var blk *data.BlockData
		var err error
		maxRetries := p.cfg.RetryCount
		for retry := uint64(0); retry <= maxRetries; retry++ {
			select {
			case <-ctx.Done():
				p.logger.Debugf("Pipeline.oneRoundWithRetries(%d): context cancelled", round)
				return
			default:
				p.logger.Debugf("Pipeline.oneRoundWithRetries(%d): retry=%d/%d", round, retry, maxRetries)
				var roundErr error
				state, blk, roundErr = p.oneRound(round, state, blk, ctx, exportSig, roundSig)
				if roundErr == nil {
					return
				}
				roundErr = fmt.Errorf("retry=%d/%d roundState=%d: %w", retry, maxRetries, state, roundErr)
				p.logger.Debugf(roundErr.Error())
				err = errors.Join(err, roundErr)
			}
			// TODO: what's the best delay here?
			time.Sleep(100 * time.Millisecond)
		}
		err = fmt.Errorf("Pipeline.oneRoundWithRetries(%d): gave up after %d attempts: %v", round, maxRetries, err)
		p.logger.Debug(err.Error())
		p.joinError(err)
		cf(newPipelineRoundError(err, round))
	}()

	return roundSig
}

type roundComplete chan struct{}

// Start pushes block data through the pipeline
func (p *pipelineImpl) Start() {
	p.logger.Debug("Pipeline.Start()")

	concurrentRounds := uint64(11) // TODO: make this configurable

	startCtx, cancelStart := context.WithCancelCause(p.ctx)

	roundSigs := make(chan roundComplete, concurrentRounds)

	round := p.pipelineMetadata.NextRound.Load()
	// initialize to a closed channel so the initial exporter is unblocked
	exportSig := make(roundComplete)
	close(exportSig)
	for i := uint64(0); i < concurrentRounds; i++ {
		exportSig = p.oneRoundWithRetries(round, exportSig, startCtx, cancelStart)
		roundSigs <- exportSig
		round++
	}

loop:
	for ; ; round++ {
		select {
		case <-p.ctx.Done():
			p.logger.Debugf("Pipeline.Start(): select <-p.ctx.Done()")
			break loop
		case importSig, ok := <-roundSigs:
			if !ok {
				p.logger.Debugf("Pipeline.Start(): select <-roundSigs: !ok")
				break loop
			}
			p.logger.Debugf("Pipeline.Start(): select <-roundSigs: ok")
			select {
			case <-p.ctx.Done():
				p.logger.Debugf("Pipeline.Start(): select <-roundSigs select <-p.ctx.Done()")
				break loop
			case <-importSig:
				p.logger.Debugf("Pipeline.Start()  select <-roundSigs select <-importSig")
				exportSig = p.oneRoundWithRetries(round, exportSig, startCtx, cancelStart)
				roundSigs <- exportSig
			}
		}
	}

	p.logger.Infof(
		"Start() finished because of <%v> with err <%v> @ Metadata.NextRound: %d",
		context.Cause(startCtx),
		p.err,
		p.pipelineMetadata.NextRound.Load(),
	)
}

func (p *pipelineImpl) oneRoundBDEPRECATED(round uint64, state roundState, prevBlk *data.BlockData, roundBcast *RoundBroadcaster) (roundState, *data.BlockData, error) {
	p.logger.Debugf("Pipeline.oneRound(): round %d, state %d", round, state)

	if state >= completed {
		return state, prevBlk, fmt.Errorf("Pipeline.oneRound(): this should never happen!!! round %d already complete: %d", round, state)
	}

	var blk data.BlockData

	if prevBlk != nil {
		blk = *prevBlk
	}

	if state <= beforePlugins {
		var err error
		// if we got here, we've already satisfied the GetBlock() pre-condition:
		if blk, err = p.importer.GetBlock(round); err != nil {
			return state, &blk, err
		}
		for _, proc := range p.processors {
			if blk, err = proc.Process(blk); err != nil {
				return state, &blk, err
			}
		}

		// only export if round @ NextRound
		// TODO: use atomic.LoadUint64()...
		p.pipelineMetadata.RoundLock()
		_ = p.pipelineMetadata.NextRoundDEPRECATED
		p.pipelineMetadata.RoundUnlock()

		roundCond := p.pipelineMetadata.roundCond
		roundCond.L.Lock()
		// Receive() constraint:
		for round > p.pipelineMetadata.NextRoundDEPRECATED {
			select {
			case <-p.ctx.Done():
				return state, &blk, p.ctx.Err()
			default:
				roundCond.Wait()
			}
		}
		roundCond.L.Unlock()

		if err = p.exporter.Receive(blk); err != nil {
			return state, &blk, err
		}
		state = beforeCallbacks
	}

	if state <= beforeCallbacks {
		for _, cb := range p.completeCallback {
			// TODO: I'm assuming that the callbacks are idempotent.
			// Is this assumption correct?
			err := cb(blk)
			if err != nil {
				return state, &blk, err
			}
		}
		roundCond := p.pipelineMetadata.roundCond
		roundCond.L.Lock()
		p.pipelineMetadata.NextRoundDEPRECATED++ // ignoring consistency checks with round
		p.logger.Debugf("Pipeline.oneRound(): round %d complete - Broadcast", round)
		roundCond.Broadcast()
		roundCond.L.Unlock()

		state = beforeMetadataSave
	}

	if state <= beforeMetadataSave {
		p.logger.Debugf("Pipeline.oneRound(%d): attempt save pipeline metadata", round)
		return state, &blk, fmt.Errorf("ARTIFICIAL ERROR round %d", round)
		// err := p.pipelineMetadata.encodeToFile(p.cfg.ConduitArgs.ConduitDataDir)
		// if err != nil {
		// 	p.logger.WithError(err).Errorf("Pipeline.oneRound(): could not save pipeline metadata")
		// 	return state, &blk, err
		// }
		// state = roundStateComplete
	}

	// if we got here, we've succeeded and there's no need to provide the block to the caller
	// for a retry so let it be garbage collected ASAP
	return state, nil, nil
}

// oneRoundWithRetriesBDEPRECATED is a goroutine for retrying oneRound() up to p.cfg.RetryCount times
func (p *pipelineImpl) oneRoundWithRetriesBDEPRECATED(round uint64, roundBcast *RoundBroadcaster, ctx context.Context, cf context.CancelCauseFunc) {
	p.logger.Debugf("Pipeline.oneRoundWithRetries(%d)", round)

	p.logger.Debugf("Pipeline.oneRoundWithRetries(%d)", round)
	var state roundState
	var blk *data.BlockData
	var err error
	maxRetries := p.cfg.RetryCount
	for retry := uint64(0); retry <= maxRetries; retry++ {
		select {
		case <-ctx.Done():
			p.logger.Debugf("Pipeline.oneRoundWithRetries(%d): context cancelled", round)
			return
		default:
			var roundErr error
			state, blk, roundErr = p.oneRoundBDEPRECATED(round, state, blk, roundBcast)
			if roundErr == nil {
				return
			}
			p.logger.WithError(roundErr).Errorf("Pipeline.oneRoundWithRetries(%d): retry=%d/%d roundState=%d", round, retry, maxRetries, state)
			err = errors.Join(err, fmt.Errorf("retry=%d/%d roundState=%d: %w", retry, maxRetries, state, roundErr))
		}
		// TODO: what's the best delay here?
		time.Sleep(100 * time.Millisecond)
	}
	err = fmt.Errorf("Pipeline.oneRoundWithRetries(%d): gave up after %d attempts: %v", round, maxRetries, err)
	p.logger.Debug(err.Error())
	cf(newPipelineRoundError(err, round))

	// these critical sections "own" the syncRound / metadata.NextRound
	// p.pipelineMetadata.RoundLock()
	// p.pipelineMetadata.NextRoundDEPRECATED = round + 1
	// p.pipelineMetadata.RoundUnlock()
	// atomic.StoreUint64(&p.pipelineMetadata.NextRoundDEPRECATED, round+1)
	// UInt64.Store(&p.pipelineMetadata.NextRoundDEPRECATED, round+1)
	p.pipelineMetadata.NextRound.Store(round + 1)
	roundBcast.Broadcast(round)
}

// Start pushes block data through the pipeline
// NOTE: this abandoned version uses the generic Broadcaster type
func (p *pipelineImpl) BStart() {
	p.logger.Debug("Pipeline.Start()")

	roundBcast := NewRoundBroadcaster(p.logger)

	startWg := sync.WaitGroup{}
	startCtx, cancelStart := context.WithCancelCause(p.ctx)

	// TODO: is this too cavelier? I.e., should I be locking the mutex?
	syncRound := p.pipelineMetadata.NextRoundDEPRECATED
	loopSubscriber := roundBcast.Subscribe()
	lookAhead := uint64(1) // TODO: make this configurable

	stop := false // TODO: is break'ing to a label more idiomatic?
	for importRound := syncRound; !stop; {
		if importRound <= syncRound+lookAhead { // launch case - update importRound
			select {
			case <-startCtx.Done():
				p.logger.Debugf("Pipeline.Start() for-if: startCtx.Done()")
				stop = true
			default:
				p.logger.Debugf("Pipeline.Start() for-if: launch oneRoundWithRetries(%d)", importRound)
				startWg.Add(1)
				go func() {
					p.oneRoundWithRetriesBDEPRECATED(importRound, roundBcast, startCtx, cancelStart)
					startWg.Done()
				}()
				importRound++
			}
		} else { // wait case - update syncRound
			var ok bool
			select {
			case <-startCtx.Done():
				p.logger.Debugf("Pipeline.Start() for-else startCtx.Done()")
				loopSubscriber.Close()
				stop = true
			case syncRound, ok = <-loopSubscriber.Chan:
				p.logger.Debugf("Pipeline.Start() for-else syncRound=%d ok=%t", syncRound, ok)
				if !ok {
					p.logger.Debugf("Pipeline.Start() loop round=%d: loopSubscriber closed", syncRound)
					cancelStart(newPipelineRoundError(fmt.Errorf("loopSubscriber closed"), syncRound))
					stop = true
				}
			}
		}
	}

	p.logger.Debugf("Pipeline.Start(): waiting for all goroutines to finish")
	startWg.Wait()
	p.logger.Infof(
		"Start() finished because of <%v> with err <%v> @ Metadata.NextRound: %d",
		context.Cause(startCtx),
		p.err,
		p.pipelineMetadata.NextRoundDEPRECATED,
	)
}

// oneRoundWithRetriesDEPRECATED is a goroutine for retrying oneRoundDEPRECATED() up to p.cfg.RetryCount times
func (p *pipelineImpl) oneRoundWithRetriesDEPRECATED(round uint64, ctx context.Context, cf context.CancelCauseFunc) {
	p.logger.Debugf("Pipeline.oneRoundWithRetriesDEPRECATED(%d)", round)
	var state roundState
	var blk *data.BlockData
	var err error
	maxRetries := p.cfg.RetryCount
	for retry := uint64(0); retry <= maxRetries; retry++ {
		select {
		case <-ctx.Done():
			p.logger.Debugf("Pipeline.oneRoundWithRetriesDEPRECATED(%d): context cancelled", round)
			return
		default:
			var roundErr error
			state, blk, roundErr = p.oneRoundDEPRECATED(round, state, blk)
			if roundErr == nil {
				return
			}
			p.logger.WithError(roundErr).Errorf("Pipeline.oneRoundWithRetriesDEPRECATED(%d): retry=%d/%d roundState=%d", round, retry, maxRetries, state)
			err = errors.Join(err, fmt.Errorf("retry=%d/%d roundState=%d: %w", retry, maxRetries, state, roundErr))
		}
		// TODO: add a sleep delay because of the error
	}
	p.logger.Debugf("Pipeline.oneRoundWithRetriesDEPRECATED(%d): gave up after %d attempts: %v", round, maxRetries, err)
	cf(newPipelineRoundError(fmt.Errorf("oneRoundWithRetriesDEPRECATED gave up after %d attempts: %w", maxRetries, err), round))
	// free up any goroutines waiting on this round
	roundCond := p.pipelineMetadata.roundCond
	roundCond.L.Lock()
	p.logger.Debugf("Pipeline.oneRoundWithRetriesDEPRECATED(%d): Broadcast for free", round)
	roundCond.Broadcast()
	roundCond.L.Unlock()
}

func (p *pipelineImpl) oneRoundDEPRECATED(round uint64, state roundState, prevBlk *data.BlockData) (roundState, *data.BlockData, error) {
	p.logger.Debugf("Pipeline.oneRoundDEPRECATED(): round %d, state %d", round, state)
	if state >= completed {
		return state, prevBlk, fmt.Errorf("Pipeline.oneRoundDEPRECATED(): this should never happen!!! round %d already complete: %d", round, state)
	}
	var blk data.BlockData
	if prevBlk != nil {
		blk = *prevBlk
	}

	if state <= beforePlugins {
		var err error
		// if we got here, we've already satisfied the GetBlock() constraint:
		if blk, err = p.importer.GetBlock(round); err != nil {
			return state, &blk, err
		}
		for _, proc := range p.processors {
			if blk, err = proc.Process(blk); err != nil {
				return state, &blk, err
			}
		}

		roundCond := p.pipelineMetadata.roundCond
		roundCond.L.Lock()
		// Receive() constraint:
		for round > p.pipelineMetadata.NextRoundDEPRECATED {
			select {
			case <-p.ctx.Done():
				return state, &blk, p.ctx.Err()
			default:
				roundCond.Wait()
			}
		}
		roundCond.L.Unlock()

		if err = p.exporter.Receive(blk); err != nil {
			return state, &blk, err
		}
		state = beforeCallbacks
	}

	if state <= beforeCallbacks {
		for _, cb := range p.completeCallback {
			// TODO: I'm assuming that the callbacks are idempotent.
			// Is this assumption correct?
			err := cb(blk)
			if err != nil {
				return state, &blk, err
			}
		}
		roundCond := p.pipelineMetadata.roundCond
		roundCond.L.Lock()
		p.pipelineMetadata.NextRoundDEPRECATED++ // ignoring consistency checks with round
		p.logger.Debugf("Pipeline.oneRoundDEPRECATED(): round %d complete - Broadcast", round)
		roundCond.Broadcast()
		roundCond.L.Unlock()

		state = beforeMetadataSave
	}

	if state <= beforeMetadataSave {
		p.logger.Debugf("Pipeline.oneRoundDEPRECATED(%d): attempt save pipeline metadata", round)
		return state, &blk, fmt.Errorf("ARTIFICIAL ERROR round %d", round)
		// err := p.pipelineMetadata.encodeToFile(p.cfg.ConduitArgs.ConduitDataDir)
		// if err != nil {
		// 	p.logger.WithError(err).Errorf("Pipeline.oneRoundDEPRECATED(): could not save pipeline metadata")
		// 	return state, &blk, err
		// }
		// state = roundStateComplete
	}

	// if we got here, we've succeeded and there's no need to provide the block to the caller
	// for a retry so let it be garbage collected ASAP
	return state, nil, nil
}

// Start pushes block data through the pipeline
func (p *pipelineImpl) DStart() {
	p.logger.Debug("Pipeline.Start()")
	p.pipelineMetadata.roundCond = sync.NewCond(&sync.Mutex{})
	defer func() {
		p.pipelineMetadata.roundCond = nil
	}()
	roundCond := p.pipelineMetadata.roundCond

	startWg := sync.WaitGroup{}
	startCtx, cancelStart := context.WithCancelCause(p.ctx)

	startWg.Add(1)
	go func() {
		p.logger.Debug("Pipeline.Start() oneRoundDEPRECATED() launcher")
		defer startWg.Done()

		roundCond.L.Lock()
		defer roundCond.L.Unlock()
		for round := p.pipelineMetadata.NextRoundDEPRECATED; true; round++ {
			round := round
			p.logger.Debugf("Pipeline.Start() oneRoundDEPRECATED() launcher loop round=%d", round)
			select {
			case <-p.ctx.Done():
				p.logger.Debugf("Pipeline.Start() oneRoundDEPRECATED() launcher loop round=%d: ctx.Done()", round)
				return
			default:
				// check the GetBlock() constraint:
				if round <= p.pipelineMetadata.NextRoundDEPRECATED+1 {
					startWg.Add(1)
					go func() {
						defer startWg.Done()
						p.oneRoundWithRetriesDEPRECATED(round, startCtx, cancelStart)
					}()
				} else {
					// for those unfamiliar with sync.Cond: Wait() unlocks the mutex
					// upon entry, only to relock it when the wait is completed
					roundCond.Wait()
				}
			}
		}
	}()

	startWg.Wait()
	p.logger.Infof(
		"Start() finished because of <%v> with err <%v> @ Metadata.NextRound: %d",
		context.Cause(startCtx),
		p.err,
		p.pipelineMetadata.NextRoundDEPRECATED,
	)
}

// CStart pushes block data through the pipeline
// "C" stands for "channel"
func (p *pipelineImpl) CStart() {
	p.wg.Add(1)
	defer p.wg.Done()

	startWg := sync.WaitGroup{}
	startCtx, cancelStart := context.WithCancelCause(p.ctx)

	p.logger.Debugf("making channels of size %d", p.chanBuffSize)
	importerChan := make(chan timedBlockData, p.chanBuffSize)
	processorChans := make([]chan timedBlockData, len(p.processors))
	for i := 0; i < len(p.processors); i++ {
		processorChans[i] = make(chan timedBlockData, p.chanBuffSize)
	}
	p.logger.Debugf("making exporterChan")

	// TODO: instead of exporterDoneSig we should: cancelStart(exporterCancelErr)
	// exporterCancelErr should be a type so that can use `errors.As` while conveying
	// more information than a hardcoded error value
	exporterDoneSig := make(chan bool)

	importerHandler := func(round uint64) (uint64 /* round */, error) {
		p.logger.Debugf("Importer starting with round: %v", round)
		imp := p.importer
		output := importerChan
		defer close(output)
		for {
			select {
			case <-startCtx.Done():
				p.logger.Debugf("Importer Done @ round: %v", round)
				return round, nil
			// TODO: won't need this signal if we follow the TODO: in exporterHandler
			case <-exporterDoneSig: // revisit, maybe just use startContext
				p.logger.Debugf("Importer exiting due to exporterDoneSig @ round: %v", round)
				return round, nil
			default:
				p.logger.Infof("Importer round: %v", round)
				importStart := time.Now()

				if blk, err := imp.GetBlock(round); err != nil {
					return round, err
				} else {
					hRound := blk.BlockHeader.Round
					if hRound != sdk.Round(round) {
						return round, fmt.Errorf("importer %s: unexpected round from block: %d", imp.Metadata().Name, hRound)
					}
					metrics.ImporterTimeSeconds.Observe(time.Since(importStart).Seconds())

					// Start time currently measures operations after block fetching is complete.
					// This is for backwards compatibility w/ Indexer's metrics
					// run through processors
					select {
					case <-startCtx.Done():
						p.logger.Debugf("Importer Done before sending @ round: %v", round)
						return round, nil
					case output <- timedBlockData{time.Now(), &blk}:
						// TODO: observe the duration waiting to send
					}
				}
			}
			round++
		}
	}

	procHandler := func(round uint64, procIdx int) (uint64 /* round */, error) {
		p.logger.Debugf("Processor[%d] starting with round: %v", procIdx, round)
		proc := p.processors[procIdx]

		var input chan timedBlockData
		if procIdx == 0 {
			input = importerChan
		} else {
			input = processorChans[procIdx-1]
		}
		output := processorChans[procIdx]
		defer close(output)

		for {
			// TODO: observe duration waiting to receive
			select {
			case <-startCtx.Done():
				p.logger.Debugf("Processor[%d] Done @ round: %v", procIdx, round)
				return round, nil
			case timedBlock, ok := <-input:
				p.logger.Infof("Processor[%d] round: %v", procIdx, round)
				if !ok {
					p.logger.Debugf("Processor[%d] channel closed @ round: %v", procIdx, round)
					return round, nil
				}
				processorStart := time.Now()
				blk := timedBlock.block
				hRound := blk.BlockHeader.Round
				if hRound != sdk.Round(round) {
					msg := fmt.Sprintf("processor[%d] (%s): unexpected round in channel block: %d", procIdx, proc.Metadata().Name, hRound)
					p.logger.Debug(msg)
					return round, fmt.Errorf(msg)
				}
				if blk, err := proc.Process(*blk); err != nil {
					p.logger.Debugf("processor[%d]: unexpected err (%v) in round: %d", procIdx, err, hRound)
					return round, err
				} else {
					hRound = blk.BlockHeader.Round
					if hRound != sdk.Round(round) {
						msg := fmt.Sprintf(
							"processor[%d] (%s): unexpected round (%d) in processed block for round: %d",
							procIdx,
							proc.Metadata().Name,
							hRound,
							round,
						)
						p.logger.Debug(msg)
						return round, fmt.Errorf(msg)
					}
					timedBlock.block = &blk
					metrics.ProcessorTimeSeconds.WithLabelValues(proc.Metadata().Name).Observe(time.Since(processorStart).Seconds())
					select {
					case <-startCtx.Done():
						p.logger.Debugf("Processor[%d] Done before sending @ round: %v", procIdx, round)
						return round, nil
					case output <- timedBlock:
						// TODO: observe the duration waiting to send
					}
				}
			}
			round++
		}
	}

	expHandler := func(round uint64) (uint64, error) {
		p.logger.Debugf("Exporter starting with round: %v", round)
		input := processorChans[len(p.processors)-1]
		defer close(exporterDoneSig)

		exp := p.exporter
		var blk *data.BlockData
		for {
			// TODO: observe duration waiting to receive
			var timedBlock timedBlockData
			var exporterStart time.Time
			select {
			case <-startCtx.Done():
				p.logger.Debugf("Exporter Done @ round: %v", round)
				return round, nil
			case timedBlock, ok := <-input:
				p.logger.Infof("Exporter round: %v", round)
				if !ok {
					p.logger.Debugf("Exporter channel closed @ round: %v", round)
					return round, nil
				}
				exporterStart = time.Now()
				blk = timedBlock.block
				hRound := blk.BlockHeader.Round
				if hRound != sdk.Round(round) {
					return round, fmt.Errorf("exporter %s: unexpected round from block: %d", exp.Metadata().Name, hRound)
				}
				if err := exp.Receive(*blk); err != nil {
					return round, err
				}
			}
			if blk == nil {
				return round, fmt.Errorf("this should never happen!!! exporter %s: unexpected nil block - cannot finishRound", exp.Metadata().Name)
			}
			err := p.finishRound(*blk, timedBlock.start, exporterStart)
			if err != nil {
				return round, err
			}
			round++
		}
	}

	// only expHandler modifies round and it hasn't yet launched
	nextRound := p.pipelineMetadata.NextRoundDEPRECATED

	// importer goroutine
	startWg.Add(1)
	go func() {
		defer startWg.Done()
		defer HandlePanic(p.logger)
		exitRound, err := importerHandler(nextRound)
		if err != nil {
			p.joinError(newPipelineRoundError(err, exitRound))
		}
		cancelStart(fmt.Errorf("Start() cancelled by importer @ round %d: %w", exitRound, err))
	}()

	// processor goroutines
	for i := 0; i < len(p.processors); i++ {
		i := i
		startWg.Add(1)
		go func() {
			defer startWg.Done()
			defer HandlePanic(p.logger)
			exitRound, err := procHandler(nextRound, i)
			if err != nil {
				p.joinError(newPipelineRoundError(err, exitRound))
			}
			cancelStart(fmt.Errorf("Start() cancelled by processor %d @ round %d: %w", i, exitRound, err))
		}()
	}

	// exporter goroutine
	startWg.Add(1)
	go func() {
		defer startWg.Done()
		defer HandlePanic(p.logger)
		exitRound, err := expHandler(nextRound)
		if err != nil {
			p.joinError(newPipelineRoundError(err, exitRound))
		}
		cancelStart(fmt.Errorf("Start() cancelled by exporter @ round %d: %w", exitRound, err))
	}()

	// TODO: add retry logic here:
	// count how many startContexts have been cancelled consecutively
	// with an error cause. Try again up to p.cfg.RetryCount times
	startWg.Wait()
	p.logger.Infof(
		"Start() finished because of <%v> with err <%v> @ Metadata.NextRound: %d",
		context.Cause(startCtx),
		p.err,
		p.pipelineMetadata.NextRoundDEPRECATED,
	)
}

// OStart pushes block data through the pipeline
// "O" stands for "original"
func (p *pipelineImpl) OStart() {
	p.wg.Add(1)
	retry := uint64(0)
	go func() {
		defer p.wg.Done()
		// We need to add a separate recover function here since it launches its own go-routine
		defer HandlePanic(p.logger)
		for {
		pipelineRun:
			metrics.PipelineRetryCount.Observe(float64(retry))
			if retry > p.cfg.RetryCount && p.cfg.RetryCount != 0 {
				p.logger.Errorf("Pipeline has exceeded maximum retry count (%d) - stopping...", p.cfg.RetryCount)
				return
			}

			if retry > 0 {
				p.logger.Infof("Retry number %d resuming after a %s retry delay.", retry, p.cfg.RetryDelay)
				time.Sleep(p.cfg.RetryDelay)
			}

			select {
			case <-p.ctx.Done():
				return
			default:
				{
					p.logger.Infof("Pipeline round: %v", p.pipelineMetadata.NextRoundDEPRECATED)
					// fetch block
					importStart := time.Now()
					blkData, err := p.importer.GetBlock(p.pipelineMetadata.NextRoundDEPRECATED)
					if err != nil {
						p.logger.Errorf("%v", err)
						p.setError(err)
						retry++
						goto pipelineRun
					}
					metrics.ImporterTimeSeconds.Observe(time.Since(importStart).Seconds())

					// TODO: Verify that the block was built with a known protocol version.

					// Start time currently measures operations after block fetching is complete.
					// This is for backwards compatibility w/ Indexer's metrics
					// run through processors
					start := time.Now()
					for _, proc := range p.processors {
						processorStart := time.Now()
						blkData, err = proc.Process(blkData)
						if err != nil {
							p.logger.Errorf("%v", err)
							p.setError(err)
							retry++
							goto pipelineRun
						}
						metrics.ProcessorTimeSeconds.WithLabelValues(proc.Metadata().Name).Observe(time.Since(processorStart).Seconds())
					}
					// run through exporter
					exporterStart := time.Now()
					err = p.exporter.Receive(blkData)
					if err != nil {
						p.logger.Errorf("%v", err)
						p.setError(err)
						retry++
						goto pipelineRun
					}
					p.finishRound(blkData, start, exporterStart)
					retry = 0
				}
			}

		}
	}()
}

// TODO: should probably be AFTER the processor not owned by it
func (p *pipelineImpl) finishRound(blkData data.BlockData, start time.Time, exporterStart time.Time) error {
	p.logger.Infof("round r=%d (%d txn) exported in %s", p.pipelineMetadata.NextRoundDEPRECATED, len(blkData.Payset), time.Since(start))

	// mutex?
	p.pipelineMetadata.NextRoundDEPRECATED++

	// shouldn't this happen AFTER p.CompleteCallback?
	err := p.pipelineMetadata.encodeToFile(p.cfg.ConduitArgs.ConduitDataDir)
	if err != nil {
		p.logger.Errorf("%v", err)
	}

	// TODO: handle these retries
	// Callback Processors
	for _, cb := range p.completeCallback {
		err = cb(blkData)
		if err != nil {
			p.logger.Errorf("%v", err)
			return err
			// p.joinError(err)
			// retry++
			// goto pipelineRun
		}
	}
	metrics.ExporterTimeSeconds.Observe(time.Since(exporterStart).Seconds())

	if p.pipelineMetadata.NextRoundDEPRECATED > 1 {
		addMetrics(blkData, time.Since(start))
	}
	p.setError(nil)
	return nil
}

func (p *pipelineImpl) Wait() {
	p.logger.Debugf("Waiting for pipeline to finish")
	start := time.Now()
	p.wg.Wait()
	p.logger.Infof("Waited for pipeline to finish for %v", time.Since(start))
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

	cancelContext, cancelFunc := context.WithCancel(ctx) // TODO: WithCancelCause()

	pipeline := &pipelineImpl{
		ctx:          cancelContext,
		cf:           cancelFunc,
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
