package pipeline

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/processors"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	logLevel   = log.ErrorLevel // log.DebugLevel // log.InfoLevel //  log.TraceLevel //
	retryCount = 3              // math.MaxUint64
)

type sleepingImporter struct {
	cfg             plugins.PluginConfig
	genesis         sdk.Genesis
	finalRound      sdk.Round
	getBlockSleep   time.Duration // when non-0, sleep when GetBlock() even in the case of an error
	returnError     bool
	onCompleteError bool
	subsystem       string
	rndOverride     uint64
	rndReqErr       error
}

func (m *sleepingImporter) Init(_ context.Context, _ data.InitProvider, cfg plugins.PluginConfig, _ *log.Logger) error {
	m.cfg = cfg
	return nil
}

func (m *sleepingImporter) GetGenesis() (*sdk.Genesis, error) {
	return &m.genesis, nil
}

func (m *sleepingImporter) Close() error {
	return nil
}

func (m *sleepingImporter) Metadata() plugins.Metadata {
	return plugins.Metadata{Name: "sleepingImporter"}
}

func (m *sleepingImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	if m.getBlockSleep > 0 {
		time.Sleep(m.getBlockSleep)
	}
	var err error
	if m.returnError {
		err = fmt.Errorf("importer")
	}
	return data.BlockData{BlockHeader: sdk.BlockHeader{Round: sdk.Round(rnd)}}, err
}

func (m *sleepingImporter) OnComplete(input data.BlockData) error {
	var err error
	if m.onCompleteError {
		err = fmt.Errorf("on complete")
	}
	m.finalRound = sdk.Round(input.BlockHeader.Round)
	return err
}

func (m *sleepingImporter) ProvideMetrics(subsystem string) []prometheus.Collector {
	m.subsystem = subsystem
	return nil
}

func (m *sleepingImporter) RoundRequest(_ plugins.PluginConfig) (uint64, error) {
	return m.rndOverride, m.rndReqErr
}

type sleepingProcessor struct {
	cfg             plugins.PluginConfig
	finalRound      sdk.Round
	processSleep    time.Duration // when non-0, sleep when Process() even in the case of an error
	returnError     bool
	onCompleteError bool
	rndOverride     uint64
	rndReqErr       error
}

func (m *sleepingProcessor) Init(_ context.Context, _ data.InitProvider, cfg plugins.PluginConfig, _ *log.Logger) error {
	m.cfg = cfg
	return nil
}

func (m *sleepingProcessor) Close() error {
	return nil
}

func (m *sleepingProcessor) RoundRequest(_ plugins.PluginConfig) (uint64, error) {
	return m.rndOverride, m.rndReqErr
}

func (m *sleepingProcessor) Metadata() plugins.Metadata {
	return plugins.Metadata{
		Name: "sleepingProcessor",
	}
}

func (m *sleepingProcessor) Process(input data.BlockData) (data.BlockData, error) {
	if m.processSleep > 0 {
		time.Sleep(m.processSleep)
	}
	var err error
	if m.returnError {
		err = fmt.Errorf("process")
	}
	return input, err
}

func (m *sleepingProcessor) OnComplete(input data.BlockData) error {
	var err error
	if m.onCompleteError {
		err = fmt.Errorf("on complete")
	}
	m.finalRound = sdk.Round(input.BlockHeader.Round)
	return err
}

type sleepingExporter struct {
	cfg             plugins.PluginConfig
	finalRound      sdk.Round
	receiveSleep    time.Duration // when non-0, sleep when Receive() even in the case of an error
	returnError     bool
	onCompleteError bool
	rndOverride     uint64
	rndReqErr       error
}

func (m *sleepingExporter) Metadata() plugins.Metadata {
	return plugins.Metadata{
		Name: "sleepingExporter",
	}
}

func (m *sleepingExporter) RoundRequest(_ plugins.PluginConfig) (uint64, error) {
	return m.rndOverride, m.rndReqErr
}

func (m *sleepingExporter) Init(_ context.Context, _ data.InitProvider, cfg plugins.PluginConfig, _ *log.Logger) error {
	m.cfg = cfg
	return nil
}

func (m *sleepingExporter) Close() error {
	return nil
}

func (m *sleepingExporter) Receive(exportData data.BlockData) error {
	if m.receiveSleep > 0 {
		time.Sleep(m.receiveSleep)
	}
	var err error
	if m.returnError {
		err = fmt.Errorf("receive")
	}
	return err
}

func (m *sleepingExporter) OnComplete(input data.BlockData) error {
	var err error
	if m.onCompleteError {
		err = fmt.Errorf("on complete")
	}
	m.finalRound = sdk.Round(input.BlockHeader.Round)
	return err
}

type benchmarkCase struct {
	name            string
	channelBuffSize int
	importerSleep   time.Duration
	processorsSleep []time.Duration
	exporterSleep   time.Duration
}

func pipeline5sec(b *testing.B, testCase benchmarkCase) int {
	benchmarkDuration := 5 * time.Second

	importer := &sleepingImporter{getBlockSleep: testCase.importerSleep}
	processors := make([]processors.Processor, len(testCase.processorsSleep))
	for i, procSleep := range testCase.processorsSleep {
		processors[i] = &sleepingProcessor{processSleep: procSleep}
	}
	exporter := &sleepingExporter{receiveSleep: testCase.exporterSleep}

	ctx, ccf := context.WithCancelCause(context.Background())

	logger := log.New()
	logger.SetLevel(logLevel)
	pImpl := pipelineImpl{
		ctx:          ctx,
		ccf:          ccf,
		logger:       logger,
		initProvider: nil,
		importer:     importer,
		processors:   processors,
		exporter:     exporter,
		pipelineMetadata: state{
			NextRound:   0,
			GenesisHash: "",
		},
		cfg: &data.Config{
			RetryDelay: 0 * time.Second,
			RetryCount: retryCount,
			ConduitArgs: &data.Args{
				ConduitDataDir: b.TempDir(),
			},
		},
	}

	pImpl.registerLifecycleCallbacks()

	// cancel the pipeline after 5 seconds
	go func() {
		time.Sleep(benchmarkDuration)
		ccf(errors.New("benchmark timeout"))
	}()

	b.StartTimer()
	pImpl.Start()
	pImpl.Wait()
	assert.NoError(b, pImpl.Error())

	fRound, err := finalRound(&pImpl)
	require.NoError(b, err)
	return int(fRound)
}

func finalRound(pi *pipelineImpl) (sdk.Round, error) {
	if mExp, ok := pi.exporter.(*sleepingExporter); ok {
		return mExp.finalRound, nil
	}
	return 0, fmt.Errorf("not a sleepingExporter: %t", pi.exporter)
}

func BenchmarkPipeline(b *testing.B) {
	benchCases := []benchmarkCase{
		{
			name:            "vanilla 2 procs without sleep",
			importerSleep:   0,
			processorsSleep: []time.Duration{0, 0},
			exporterSleep:   0,
		},
		{
			name:            "uniform sleep of 10ms",
			importerSleep:   10 * time.Millisecond,
			processorsSleep: []time.Duration{10 * time.Millisecond, 10 * time.Millisecond},
			exporterSleep:   10 * time.Millisecond,
		},
		{
			name:            "exporter 10ms while others 1ms",
			importerSleep:   time.Millisecond,
			processorsSleep: []time.Duration{time.Millisecond, time.Millisecond},
			exporterSleep:   10 * time.Millisecond,
		},
		{
			name:            "importer 10ms while others 1ms",
			importerSleep:   10 * time.Millisecond,
			processorsSleep: []time.Duration{time.Millisecond, time.Millisecond},
			exporterSleep:   time.Millisecond,
		},
		{
			name:            "first processor 10ms while others 1ms",
			importerSleep:   time.Millisecond,
			processorsSleep: []time.Duration{10 * time.Millisecond, time.Millisecond},
			exporterSleep:   time.Millisecond,
		},
	}
	for _, buffSize := range []int{1} {
		for _, bc := range benchCases {
			bc.channelBuffSize = buffSize
			b.Run(fmt.Sprintf("%s-size-%d", bc.name, bc.channelBuffSize), func(b *testing.B) {
				rounds := 0
				for i := 0; i < b.N; i++ {
					rounds += pipeline5sec(b, bc)
				}
				secs := b.Elapsed().Seconds()
				rps := float64(rounds) / secs
				b.ReportMetric(rps, "rounds/sec")
			})
		}
	}
}
