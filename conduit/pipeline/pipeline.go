package pipeline

import (
	"context"
	"encoding/base64"
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
	"github.com/algorand/indexer/util"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/metrics"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters"
	"github.com/algorand/conduit/conduit/plugins/importers"
	"github.com/algorand/conduit/conduit/plugins/processors"
)

type ErrOverrideConflict struct {
	pluginOrCLI uint64
	other       uint64
	cli         bool
}

func MakeErrOverrideConflict(pluginOrCLI, other uint64, cli bool) error {
	return ErrOverrideConflict{
		pluginOrCLI: pluginOrCLI,
		other:       other,
		cli:         cli,
	}
}

func (e ErrOverrideConflict) Error() string {
	if e.cli {
		return fmt.Sprintf("inconsistent round overrides detected: command line (%d), plugins (%d)", e.pluginOrCLI, e.other)

	} else {
		return fmt.Sprintf("inconsistent round overrides detected: %d, %d", e.pluginOrCLI, e.other)
	}
}

// NameConfigPair is a generic structure used across plugin configuration ser/de
type NameConfigPair struct {
	Name   string                 `yaml:"name"`
	Config map[string]interface{} `yaml:"config"`
}

// Metrics configs for turning on Prometheus endpoint /metrics
type Metrics struct {
	Mode   string `yaml:"mode"`
	Addr   string `yaml:"addr"`
	Prefix string `yaml:"prefix"`
}

// Config stores configuration specific to the conduit pipeline
type Config struct {
	// ConduitArgs are the program inputs. Should not be serialized for config.
	ConduitArgs *conduit.Args `yaml:"-"`

	CPUProfile  string `yaml:"cpu-profile"`
	PIDFilePath string `yaml:"pid-filepath"`
	HideBanner  bool   `yaml:"hide-banner"`

	LogFile  string `yaml:"log-file"`
	LogLevel string `yaml:"log-level"`
	// Store a local copy to access parent variables
	Importer   NameConfigPair   `yaml:"importer"`
	Processors []NameConfigPair `yaml:"processors"`
	Exporter   NameConfigPair   `yaml:"exporter"`
	Metrics    Metrics          `yaml:"metrics"`
	// RetryCount is the number of retries to perform for an error in the pipeline
	RetryCount uint64 `yaml:"retry-count"`
	// RetryDelay is a duration amount interpreted from a string
	RetryDelay time.Duration `yaml:"retry-delay"`
}

// Valid validates pipeline config
func (cfg *Config) Valid() error {
	if cfg.ConduitArgs == nil {
		return fmt.Errorf("Args.Valid(): conduit args were nil")
	}

	// If it is a negative time, it is an error
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("Args.Valid(): invalid retry delay - time duration was negative (%s)", cfg.RetryDelay.String())
	}

	return nil
}

// MakePipelineConfig creates a pipeline configuration
func MakePipelineConfig(args *conduit.Args) (*Config, error) {
	if args == nil {
		return nil, fmt.Errorf("MakePipelineConfig(): empty conduit config")
	}

	if !util.IsDir(args.ConduitDataDir) {
		return nil, fmt.Errorf("MakePipelineConfig(): invalid data dir '%s'", args.ConduitDataDir)
	}

	// Search for pipeline configuration in data directory
	autoloadParamConfigPath, err := util.GetConfigFromDataDir(args.ConduitDataDir, conduit.DefaultConfigBaseName, []string{"yml", "yaml"})
	if err != nil || autoloadParamConfigPath == "" {
		return nil, fmt.Errorf("MakePipelineConfig(): could not find %s in data directory (%s)", conduit.DefaultConfigName, args.ConduitDataDir)
	}

	file, err := os.Open(autoloadParamConfigPath)
	if err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): reading config error: %w", err)
	}

	pCfgDecoder := yaml.NewDecoder(file)
	// Make sure we are strict about only unmarshalling known fields
	pCfgDecoder.KnownFields(true)

	var pCfg Config
	// Set default value for retry variables
	pCfg.RetryDelay = 1 * time.Second
	pCfg.RetryCount = 10
	err = pCfgDecoder.Decode(&pCfg)
	if err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): config file (%s) was mal-formed yaml: %w", autoloadParamConfigPath, err)
	}

	// For convenience, include the command line arguments.
	pCfg.ConduitArgs = args

	// Default log level.
	if pCfg.LogLevel == "" {
		pCfg.LogLevel = conduit.DefaultLogLevel.String()
	}

	if err := pCfg.Valid(); err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): config file (%s) had mal-formed schema: %w", autoloadParamConfigPath, err)
	}

	return &pCfg, nil
}

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

type pipelineImpl struct {
	ctx      context.Context
	cf       context.CancelFunc
	wg       sync.WaitGroup
	cfg      *Config
	logger   *log.Logger
	profFile *os.File
	err      error
	mu       sync.RWMutex

	initProvider *data.InitProvider

	importer         *importers.Importer
	processors       []*processors.Processor
	exporter         *exporters.Exporter
	completeCallback []conduit.OnCompleteFunc

	pipelineMetadata state
}

func (p *pipelineImpl) Error() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.err
}

func (p *pipelineImpl) setError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
}

func (p *pipelineImpl) registerLifecycleCallbacks() {
	if v, ok := (*p.importer).(conduit.Completed); ok {
		p.completeCallback = append(p.completeCallback, v.OnComplete)
	}
	for _, processor := range p.processors {
		if v, ok := (*processor).(conduit.Completed); ok {
			p.completeCallback = append(p.completeCallback, v.OnComplete)
		}
	}
	if v, ok := (*p.exporter).(conduit.Completed); ok {
		p.completeCallback = append(p.completeCallback, v.OnComplete)
	}
}

func (p *pipelineImpl) registerPluginMetricsCallbacks() {
	var collectors []prometheus.Collector
	if v, ok := (*p.importer).(conduit.PluginMetrics); ok {
		collectors = append(collectors, v.ProvideMetrics(p.cfg.Metrics.Prefix)...)
	}
	for _, processor := range p.processors {
		if v, ok := (*processor).(conduit.PluginMetrics); ok {
			collectors = append(collectors, v.ProvideMetrics(p.cfg.Metrics.Prefix)...)
		}
	}
	if v, ok := (*p.exporter).(conduit.PluginMetrics); ok {
		collectors = append(collectors, v.ProvideMetrics(p.cfg.Metrics.Prefix)...)
	}
	for _, c := range collectors {
		_ = prometheus.Register(c)
	}
}

func (p *pipelineImpl) makeConfig(cfg NameConfigPair, pluginType plugins.PluginType) (*log.Logger, plugins.PluginConfig, error) {
	configs, err := yaml.Marshal(cfg.Config)
	if err != nil {
		return nil, plugins.PluginConfig{}, fmt.Errorf("makeConfig(): could not serialize config: %w", err)
	}

	l := log.New()
	l.SetOutput(p.logger.Out)
	l.SetFormatter(makePluginLogFormatter(plugins.Processor, cfg.Name))

	var config plugins.PluginConfig
	config.Config = string(configs)
	if p.cfg != nil && p.cfg.ConduitArgs != nil {
		config.DataDir = path.Join(p.cfg.ConduitArgs.ConduitDataDir, fmt.Sprintf("%s_%s", pluginType, cfg.Name))
		err = os.MkdirAll(config.DataDir, os.ModePerm)
		if err != nil {
			return nil, plugins.PluginConfig{}, fmt.Errorf("makeConfig: unable to create plugin data directory: %w", err)
		}
	}

	return l, config, nil
}

// pluginRoundOverride looks at the round override argument, and attempts to query
// each plugin for a requested round override. If there is no override, 0 is
// returned, if there is one or more round override which are in agreement,
// that round is returned, if there are multiple different round overrides
// an error is returned.
func (p *pipelineImpl) pluginRoundOverride() (uint64, error) {
	var pluginOverride uint64

	if v, ok := (*p.importer).(conduit.RoundRequestor); ok {
		_, config, err := p.makeConfig(p.cfg.Importer, plugins.Importer)
		if err != nil {
			return 0, err
		}
		rnd := v.RoundRequest(config)
		if pluginOverride != 0 && rnd != 0 && rnd != pluginOverride {
			return 0, MakeErrOverrideConflict(pluginOverride, rnd, false)
		}
		if rnd != 0 {
			pluginOverride = rnd
		}
	}
	for idx, processor := range p.processors {
		if v, ok := (*processor).(conduit.RoundRequestor); ok {
			_, config, err := p.makeConfig(p.cfg.Processors[idx], plugins.Processor)
			if err != nil {
				return 0, err
			}
			rnd := v.RoundRequest(config)
			if pluginOverride != 0 && rnd != 0 && rnd != pluginOverride {
				return 0, MakeErrOverrideConflict(pluginOverride, rnd, false)
			}
			if rnd != 0 {
				pluginOverride = rnd
			}
		}
	}
	if v, ok := (*p.exporter).(conduit.RoundRequestor); ok {
		_, config, err := p.makeConfig(p.cfg.Importer, plugins.Exporter)
		if err != nil {
			return 0, err
		}
		rnd := v.RoundRequest(config)
		if pluginOverride != 0 && rnd != 0 && rnd != pluginOverride {
			return 0, MakeErrOverrideConflict(pluginOverride, rnd, false)
		}
		if rnd != 0 {
			pluginOverride = rnd
		}
	}

	// Check command line arg
	if pluginOverride != 0 && p.cfg.ConduitArgs.NextRoundOverride != 0 && p.cfg.ConduitArgs.NextRoundOverride != pluginOverride {
		return 0, MakeErrOverrideConflict(p.cfg.ConduitArgs.NextRoundOverride, pluginOverride, true)
	}
	if p.cfg.ConduitArgs.NextRoundOverride != 0 {
		pluginOverride = p.cfg.ConduitArgs.NextRoundOverride
	}

	return pluginOverride, nil
}

// Init prepares the pipeline for processing block data
func (p *pipelineImpl) Init() error {
	p.logger.Infof("Starting Pipeline Initialization")

	if p.cfg.Metrics.Prefix == "" {
		p.cfg.Metrics.Prefix = conduit.DefaultMetricsPrefix
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
		err := util.CreateIndexerPidFile(p.logger, p.cfg.PIDFilePath)
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
	pluginRoundOverride, err := p.pluginRoundOverride()
	if err != nil {
		return fmt.Errorf("Pipeline.Init(): error resolving plugin round override: %w", err)
	}
	if pluginRoundOverride > 0 {
		p.logger.Infof("Overriding default next round from %d to %d.", p.pipelineMetadata.NextRound, p.cfg.ConduitArgs.NextRoundOverride)
		p.pipelineMetadata.NextRound = pluginRoundOverride
	}

	// InitProvider
	round := sdk.Round(p.pipelineMetadata.NextRound)
	// Initial genesis object is nil--gets updated after importer.Init
	var initProvider data.InitProvider = conduit.MakePipelineInitProvider(&round, nil)
	p.initProvider = &initProvider

	// Initialize Importer
	{
		importerLogger, pluginConfig, err := p.makeConfig(p.cfg.Importer, plugins.Importer)
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): could not make %s config: %w", p.cfg.Importer.Name, err)
		}
		genesis, err := (*p.importer).Init(p.ctx, *p.initProvider, pluginConfig, importerLogger)
		if err != nil {
			return fmt.Errorf("Pipeline.Init(): could not initialize importer (%s): %w", p.cfg.Importer.Name, err)
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
		err = (*processor).Init(p.ctx, *p.initProvider, config, logger)
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
		err = (*p.exporter).Init(p.ctx, *p.initProvider, config, logger)
		if err != nil {
			return fmt.Errorf("Pipeline.Start(): could not initialize Exporter (%s): %w", p.cfg.Exporter.Name, err)
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

	if err := (*p.importer).Close(); err != nil {
		// Log and continue on closing the rest of the pipeline
		p.logger.Errorf("Pipeline.Stop(): Importer (%s) error on close: %v", (*p.importer).Metadata().Name, err)
	}

	for _, processor := range p.processors {
		if err := (*processor).Close(); err != nil {
			// Log and continue on closing the rest of the pipeline
			p.logger.Errorf("Pipeline.Stop(): Processor (%s) error on close: %v", (*processor).Metadata().Name, err)
		}
	}

	if err := (*p.exporter).Close(); err != nil {
		p.logger.Errorf("Pipeline.Stop(): Exporter (%s) error on close: %v", (*p.exporter).Metadata().Name, err)
	}
}

func (p *pipelineImpl) addMetrics(block data.BlockData, importTime time.Duration) {
	metrics.BlockImportTimeSeconds.Observe(importTime.Seconds())
	metrics.ImportedTxnsPerBlock.Observe(float64(len(block.Payset)))
	metrics.ImportedRoundGauge.Set(float64(block.Round()))
	txnCountByType := make(map[string]int)
	for _, txn := range block.Payset {
		txnCountByType[string(txn.Txn.Type)]++
	}
	for k, v := range txnCountByType {
		metrics.ImportedTxns.WithLabelValues(k).Set(float64(v))
	}
}

// Start pushes block data through the pipeline
func (p *pipelineImpl) Start() {
	p.wg.Add(1)
	retry := uint64(0)
	go func() {
		defer p.wg.Done()
		// We need to add a separate recover function here since it launches its own go-routine
		defer HandlePanic(p.logger)
		for {
		pipelineRun:
			metrics.PipelineRetryCount.Observe(float64(retry))
			if retry > p.cfg.RetryCount {
				p.logger.Errorf("Pipeline has exceeded maximum retry count (%d) - stopping...", p.cfg.RetryCount)
				return
			}

			if retry > 0 {
				time.Sleep(p.cfg.RetryDelay)
			}

			select {
			case <-p.ctx.Done():
				return
			default:
				{
					p.logger.Infof("Pipeline round: %v", p.pipelineMetadata.NextRound)
					// fetch block
					importStart := time.Now()
					blkData, err := (*p.importer).GetBlock(p.pipelineMetadata.NextRound)
					if err != nil {
						p.logger.Errorf("%v", err)
						p.setError(err)
						retry++
						goto pipelineRun
					}
					metrics.ImporterTimeSeconds.Observe(time.Since(importStart).Seconds())

					// TODO: Verify that the block was build with a known protocol version.

					// Start time currently measures operations after block fetching is complete.
					// This is for backwards compatibility w/ Indexer's metrics
					// run through processors
					start := time.Now()
					for _, proc := range p.processors {
						processorStart := time.Now()
						blkData, err = (*proc).Process(blkData)
						if err != nil {
							p.logger.Errorf("%v", err)
							p.setError(err)
							retry++
							goto pipelineRun
						}
						metrics.ProcessorTimeSeconds.WithLabelValues((*proc).Metadata().Name).Observe(time.Since(processorStart).Seconds())
					}
					// run through exporter
					exporterStart := time.Now()
					err = (*p.exporter).Receive(blkData)
					if err != nil {
						p.logger.Errorf("%v", err)
						p.setError(err)
						retry++
						goto pipelineRun
					}
					p.logger.Infof("round r=%d (%d txn) exported in %s", p.pipelineMetadata.NextRound, len(blkData.Payset), time.Since(start))

					// Increment Round, update metadata
					p.pipelineMetadata.NextRound++
					err = p.pipelineMetadata.encodeToFile(p.cfg.ConduitArgs.ConduitDataDir)
					if err != nil {
						p.logger.Errorf("%v", err)
					}

					// Callback Processors
					for _, cb := range p.completeCallback {
						err = cb(blkData)
						if err != nil {
							p.logger.Errorf("%v", err)
							p.setError(err)
							retry++
							goto pipelineRun
						}
					}
					metrics.ExporterTimeSeconds.Observe(time.Since(exporterStart).Seconds())
					// Ignore round 0 (which is empty).
					if p.pipelineMetadata.NextRound > 1 {
						p.addMetrics(blkData, time.Since(start))
					}
					p.setError(nil)
					retry = 0
				}
			}

		}
	}()
}

func (p *pipelineImpl) Wait() {
	p.wg.Wait()
}

// start a http server serving /metrics
func (p *pipelineImpl) startMetricsServer() {
	http.Handle("/metrics", promhttp.Handler())
	_ = http.ListenAndServe(p.cfg.Metrics.Addr, nil)
	p.logger.Infof("conduit metrics serving on %s", p.cfg.Metrics.Addr)
}

// MakePipeline creates a Pipeline
func MakePipeline(ctx context.Context, cfg *Config, logger *log.Logger) (Pipeline, error) {

	if cfg == nil {
		return nil, fmt.Errorf("MakePipeline(): pipeline config was empty")
	}

	if err := cfg.Valid(); err != nil {
		return nil, fmt.Errorf("MakePipeline(): %w", err)
	}

	if logger == nil {
		return nil, fmt.Errorf("MakePipeline(): logger was empty")
	}

	cancelContext, cancelFunc := context.WithCancel(ctx)

	pipeline := &pipelineImpl{
		ctx:          cancelContext,
		cf:           cancelFunc,
		cfg:          cfg,
		logger:       logger,
		initProvider: nil,
		importer:     nil,
		processors:   []*processors.Processor{},
		exporter:     nil,
	}

	importerName := cfg.Importer.Name

	importerBuilder, err := importers.ImporterBuilderByName(importerName)
	if err != nil {
		return nil, fmt.Errorf("MakePipeline(): could not build importer '%s': %w", importerName, err)
	}

	importer := importerBuilder.New()
	pipeline.importer = &importer
	logger.Infof("Found Importer: %s", importerName)

	// ---

	for _, processorConfig := range cfg.Processors {
		processorName := processorConfig.Name

		processorBuilder, err := processors.ProcessorBuilderByName(processorName)
		if err != nil {
			return nil, fmt.Errorf("MakePipeline(): could not build processor '%s': %w", processorName, err)
		}

		processor := processorBuilder.New()
		pipeline.processors = append(pipeline.processors, &processor)
		logger.Infof("Found Processor: %s", processorName)
	}

	// ---

	exporterName := cfg.Exporter.Name

	exporterBuilder, err := exporters.ExporterBuilderByName(exporterName)
	if err != nil {
		return nil, fmt.Errorf("MakePipeline(): could not build exporter '%s': %w", exporterName, err)
	}

	exporter := exporterBuilder.New()
	pipeline.exporter = &exporter
	logger.Infof("Found Exporter: %s", exporterName)

	return pipeline, nil
}
