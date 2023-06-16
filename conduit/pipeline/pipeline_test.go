package pipeline

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/metrics"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters"
	"github.com/algorand/conduit/conduit/plugins/importers"
	"github.com/algorand/conduit/conduit/plugins/processors"
	"github.com/algorand/conduit/conduit/telemetry"
)

// a unique block data to validate with tests
var uniqueBlockData = data.BlockData{
	BlockHeader: sdk.BlockHeader{
		Round: 1337,
	},
}

type mockImporter struct {
	mock.Mock
	importers.Importer
	cfg             plugins.PluginConfig
	genesis         sdk.Genesis
	finalRound      sdk.Round
	returnError     bool
	onCompleteError bool
	subsystem       string
	rndOverride     uint64
	rndReqErr       error
}

func (m *mockImporter) Init(_ context.Context, _ data.InitProvider, cfg plugins.PluginConfig, _ *log.Logger) error {
	m.cfg = cfg
	return nil
}

func (m *mockImporter) GetGenesis() (*sdk.Genesis, error) {
	return &m.genesis, nil
}

func (m *mockImporter) Close() error {
	return nil
}

func (m *mockImporter) Metadata() plugins.Metadata {
	return plugins.Metadata{Name: "mockImporter"}
}

func (m *mockImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	var err error
	if m.returnError {
		err = fmt.Errorf("importer")
	}
	m.Called(rnd)
	// Return an error to make sure we
	return uniqueBlockData, err
}

func (m *mockImporter) OnComplete(input data.BlockData) error {
	var err error
	if m.onCompleteError {
		err = fmt.Errorf("on complete")
	}
	m.finalRound = sdk.Round(input.BlockHeader.Round)
	m.Called(input)
	return err
}

func (m *mockImporter) ProvideMetrics(subsystem string) []prometheus.Collector {
	m.subsystem = subsystem
	return nil
}

func (m *mockImporter) RoundRequest(_ plugins.PluginConfig) (uint64, error) {
	return m.rndOverride, m.rndReqErr
}

type mockProcessor struct {
	mock.Mock
	processors.Processor
	cfg             plugins.PluginConfig
	finalRound      sdk.Round
	returnError     bool
	onCompleteError bool
	rndOverride     uint64
	rndReqErr       error
}

func (m *mockProcessor) Init(_ context.Context, _ data.InitProvider, cfg plugins.PluginConfig, _ *log.Logger) error {
	m.cfg = cfg
	return nil
}

func (m *mockProcessor) Close() error {
	return nil
}

func (m *mockProcessor) RoundRequest(_ plugins.PluginConfig) (uint64, error) {
	return m.rndOverride, m.rndReqErr
}

func (m *mockProcessor) Metadata() plugins.Metadata {
	return plugins.Metadata{
		Name: "mockProcessor",
	}
}

func (m *mockProcessor) Process(input data.BlockData) (data.BlockData, error) {
	var err error
	if m.returnError {
		err = fmt.Errorf("process")
	}
	m.Called(input)
	input.BlockHeader.Round++
	return input, err
}

func (m *mockProcessor) OnComplete(input data.BlockData) error {
	var err error
	if m.onCompleteError {
		err = fmt.Errorf("on complete")
	}
	m.finalRound = sdk.Round(input.BlockHeader.Round)
	m.Called(input)
	return err
}

type mockExporter struct {
	mock.Mock
	exporters.Exporter
	cfg             plugins.PluginConfig
	finalRound      sdk.Round
	returnError     bool
	onCompleteError bool
	rndOverride     uint64
	rndReqErr       error
}

func (m *mockExporter) Metadata() plugins.Metadata {
	return plugins.Metadata{
		Name: "mockExporter",
	}
}

func (m *mockExporter) RoundRequest(_ plugins.PluginConfig) (uint64, error) {
	return m.rndOverride, m.rndReqErr
}

func (m *mockExporter) Init(_ context.Context, _ data.InitProvider, cfg plugins.PluginConfig, _ *log.Logger) error {
	m.cfg = cfg
	return nil
}

func (m *mockExporter) Close() error {
	return nil
}

func (m *mockExporter) Receive(exportData data.BlockData) error {
	var err error
	if m.returnError {
		err = fmt.Errorf("receive")
	}
	m.Called(exportData)
	return err
}

func (m *mockExporter) OnComplete(input data.BlockData) error {
	var err error
	if m.onCompleteError {
		err = fmt.Errorf("on complete")
	}
	m.finalRound = sdk.Round(input.BlockHeader.Round)
	m.Called(input)
	return err
}

func mockPipeline(t *testing.T, dataDir string) (*pipelineImpl, *test.Hook, *mockImporter, *mockProcessor, *mockExporter) {
	if dataDir == "" {
		dataDir = t.TempDir()
	}
	mImporter := &mockImporter{genesis: sdk.Genesis{Network: "test"}}
	var pImporter importers.Importer = mImporter
	mProcessor := &mockProcessor{}
	var pProcessor processors.Processor = mProcessor
	mExporter := &mockExporter{}
	var pExporter exporters.Exporter = mExporter

	l, hook := test.NewNullLogger()
	pImpl := pipelineImpl{
		cfg: &data.Config{
			ConduitArgs: &data.Args{
				ConduitDataDir:    dataDir,
				NextRoundOverride: 0,
			},
			Importer: data.NameConfigPair{
				Name:   "mockImporter",
				Config: map[string]interface{}{},
			},
			Processors: []data.NameConfigPair{
				{
					Name:   "mockProcessor",
					Config: map[string]interface{}{},
				},
			},
			Exporter: data.NameConfigPair{
				Name:   "mockExporter",
				Config: map[string]interface{}{},
			},
		},
		logger:       l,
		initProvider: nil,
		importer:     &pImporter,
		processors:   []*processors.Processor{&pProcessor},
		exporter:     &pExporter,
		pipelineMetadata: state{
			GenesisHash: "",
			Network:     "",
			NextRound:   3,
		},
	}

	return &pImpl, hook, mImporter, mProcessor, mExporter
}

// TestPipelineRun tests that running the pipeline calls the correct functions with mocking
func TestPipelineRun(t *testing.T) {
	mImporter := mockImporter{}
	mImporter.On("GetBlock", mock.Anything).Return(uniqueBlockData, nil)
	mProcessor := mockProcessor{}
	processorData := uniqueBlockData
	processorData.BlockHeader.Round++
	mProcessor.On("Process", mock.Anything).Return(processorData)
	mProcessor.On("OnComplete", mock.Anything).Return(nil)
	mExporter := mockExporter{}
	mExporter.On("Receive", mock.Anything).Return(nil)

	var pImporter importers.Importer = &mImporter
	var pProcessor processors.Processor = &mProcessor
	var pExporter exporters.Exporter = &mExporter
	var cbComplete conduit.Completed = &mProcessor

	ctx, cf := context.WithCancel(context.Background())

	l, _ := test.NewNullLogger()
	pImpl := pipelineImpl{
		ctx:              ctx,
		cf:               cf,
		logger:           l,
		initProvider:     nil,
		importer:         &pImporter,
		processors:       []*processors.Processor{&pProcessor},
		exporter:         &pExporter,
		completeCallback: []conduit.OnCompleteFunc{cbComplete.OnComplete},
		pipelineMetadata: state{
			NextRound:   0,
			GenesisHash: "",
		},
		cfg: &data.Config{
			RetryDelay: 0 * time.Second,
			RetryCount: math.MaxUint64,
			ConduitArgs: &data.Args{
				ConduitDataDir: t.TempDir(),
			},
		},
	}

	go func() {
		time.Sleep(1 * time.Second)
		cf()
	}()

	pImpl.Start()
	pImpl.Wait()
	assert.NoError(t, pImpl.Error())

	assert.Equal(t, mProcessor.finalRound, uniqueBlockData.BlockHeader.Round+1)

	mock.AssertExpectationsForObjects(t, &mImporter, &mProcessor, &mExporter)

}

// TestPipelineCpuPidFiles tests that cpu and pid files are created when specified
func TestPipelineCpuPidFiles(t *testing.T) {

	tempDir := t.TempDir()
	pidFilePath := filepath.Join(tempDir, "pidfile")
	cpuFilepath := filepath.Join(tempDir, "cpufile")

	pImpl, _, _, _, _ := mockPipeline(t, tempDir)

	err := pImpl.Init()
	assert.NoError(t, err)

	// Test that file is not created
	_, err = os.Stat(pidFilePath)
	assert.ErrorIs(t, err, os.ErrNotExist)

	_, err = os.Stat(cpuFilepath)
	assert.ErrorIs(t, err, os.ErrNotExist)

	// Test that they were created

	pImpl.cfg.PIDFilePath = pidFilePath
	pImpl.cfg.CPUProfile = cpuFilepath

	err = pImpl.Init()
	assert.NoError(t, err)

	// Test that file is created
	_, err = os.Stat(cpuFilepath)
	assert.Nil(t, err)

	_, err = os.Stat(pidFilePath)
	assert.Nil(t, err)
}

// TestPipelineErrors tests the pipeline erroring out at different stages
func TestPipelineErrors(t *testing.T) {
	tempDir := t.TempDir()

	mImporter := mockImporter{}
	mImporter.On("GetBlock", mock.Anything).Return(uniqueBlockData, nil)
	mProcessor := mockProcessor{}
	processorData := uniqueBlockData
	processorData.BlockHeader.Round++
	mProcessor.On("Process", mock.Anything).Return(processorData)
	mProcessor.On("OnComplete", mock.Anything).Return(nil)
	mExporter := mockExporter{}
	mExporter.On("Receive", mock.Anything).Return(nil)

	var pImporter importers.Importer = &mImporter
	var pProcessor processors.Processor = &mProcessor
	var pExporter exporters.Exporter = &mExporter
	var cbComplete conduit.Completed = &mProcessor

	ctx, cf := context.WithCancel(context.Background())
	l, _ := test.NewNullLogger()
	pImpl := pipelineImpl{
		ctx: ctx,
		cf:  cf,
		cfg: &data.Config{
			RetryDelay: 0 * time.Second,
			RetryCount: math.MaxUint64,
			ConduitArgs: &data.Args{
				ConduitDataDir: tempDir,
			},
		},
		logger:           l,
		initProvider:     nil,
		importer:         &pImporter,
		processors:       []*processors.Processor{&pProcessor},
		exporter:         &pExporter,
		completeCallback: []conduit.OnCompleteFunc{cbComplete.OnComplete},
		pipelineMetadata: state{},
	}

	mImporter.returnError = true

	go pImpl.Start()
	time.Sleep(time.Millisecond)
	pImpl.cf()
	pImpl.Wait()
	assert.Error(t, pImpl.Error(), fmt.Errorf("importer"))

	mImporter.returnError = false
	mProcessor.returnError = true
	pImpl.ctx, pImpl.cf = context.WithCancel(context.Background())
	pImpl.setError(nil)
	go pImpl.Start()
	time.Sleep(time.Millisecond)
	pImpl.cf()
	pImpl.Wait()
	assert.Error(t, pImpl.Error(), fmt.Errorf("process"))

	mProcessor.returnError = false
	mProcessor.onCompleteError = true
	pImpl.ctx, pImpl.cf = context.WithCancel(context.Background())
	pImpl.setError(nil)
	go pImpl.Start()
	time.Sleep(time.Millisecond)
	pImpl.cf()
	pImpl.Wait()
	assert.Error(t, pImpl.Error(), fmt.Errorf("on complete"))

	mProcessor.onCompleteError = false
	mExporter.returnError = true
	pImpl.ctx, pImpl.cf = context.WithCancel(context.Background())
	pImpl.setError(nil)
	go pImpl.Start()
	time.Sleep(time.Millisecond)
	pImpl.cf()
	pImpl.Wait()
	assert.Error(t, pImpl.Error(), fmt.Errorf("exporter"))
}

func Test_pipelineImpl_registerLifecycleCallbacks(t *testing.T) {
	mImporter := mockImporter{}
	mImporter.On("GetBlock", mock.Anything).Return(uniqueBlockData, nil)
	mProcessor := mockProcessor{}
	processorData := uniqueBlockData
	processorData.BlockHeader.Round++
	mProcessor.On("Process", mock.Anything).Return(processorData)
	mProcessor.On("OnComplete", mock.Anything).Return(nil)
	mExporter := mockExporter{}
	mExporter.On("Receive", mock.Anything).Return(nil)

	var pImporter importers.Importer = &mImporter
	var pProcessor processors.Processor = &mProcessor
	var pExporter exporters.Exporter = &mExporter

	ctx, cf := context.WithCancel(context.Background())
	l, _ := test.NewNullLogger()
	pImpl := pipelineImpl{
		ctx:          ctx,
		cf:           cf,
		cfg:          &data.Config{},
		logger:       l,
		initProvider: nil,
		importer:     &pImporter,
		processors:   []*processors.Processor{&pProcessor, &pProcessor},
		exporter:     &pExporter,
	}

	// Each plugin implements the Completed interface, so there should be 4
	// plugins registered (one of them is registered twice)
	pImpl.registerLifecycleCallbacks()
	assert.Len(t, pImpl.completeCallback, 4)
}

// TestBlockMetaDataFile tests that metadata.json file is created as expected
func TestPluginConfigDataDir(t *testing.T) {
	datadir := t.TempDir()
	pImpl, _, mImporter, mProcessor, mExporter := mockPipeline(t, datadir)

	err := pImpl.Init()
	assert.NoError(t, err)

	assert.Equal(t, path.Join(datadir, "importer_mockImporter"), mImporter.cfg.DataDir)
	assert.DirExists(t, mImporter.cfg.DataDir)
	assert.Equal(t, path.Join(datadir, "processor_mockProcessor"), mProcessor.cfg.DataDir)
	assert.DirExists(t, mProcessor.cfg.DataDir)
	assert.Equal(t, path.Join(datadir, "exporter_mockExporter"), mExporter.cfg.DataDir)
	assert.DirExists(t, mExporter.cfg.DataDir)
}

func TestGenesisHash(t *testing.T) {
	datadir := t.TempDir()
	pImpl, _, _, _, _ := mockPipeline(t, datadir)

	// write genesis hash to metadata.json
	err := pImpl.Init()
	assert.NoError(t, err)

	// read genesis hash from metadata.json
	blockmetaData, err := readBlockMetadata(datadir)
	assert.NoError(t, err)
	genesis := &sdk.Genesis{Network: "test"}
	gh := genesis.Hash()
	assert.Equal(t, base64.StdEncoding.EncodeToString(gh[:]), blockmetaData.GenesisHash)
	assert.Equal(t, "test", blockmetaData.Network)

	// mock a different genesis hash
	var pImporter importers.Importer = &mockImporter{genesis: sdk.Genesis{Network: "dev"}}
	pImpl.importer = &pImporter
	err = pImpl.Init()
	assert.Contains(t, err.Error(), "genesis hash in metadata does not match")
}

func TestPipelineMetricsConfigs(t *testing.T) {
	pImpl, _, _, _, _ := mockPipeline(t, "")

	getMetrics := func() (*http.Response, error) {
		resp0, err0 := http.Get(fmt.Sprintf("http://localhost%s/metrics", pImpl.cfg.Metrics.Addr))
		return resp0, err0
	}
	// metrics should be OFF by default
	err := pImpl.Init()
	assert.NoError(t, err)
	time.Sleep(1 * time.Second)
	_, err = getMetrics()
	assert.Error(t, err)

	// metrics mode OFF, default prefix
	pImpl.cfg.Metrics = data.Metrics{
		Mode: "OFF",
		Addr: ":8081",
	}
	pImpl.Init()
	time.Sleep(1 * time.Second)
	_, err = getMetrics()
	assert.Error(t, err)
	assert.Equal(t, pImpl.cfg.Metrics.Prefix, "conduit")

	// metrics mode ON, override prefix
	prefixOverride := "asdfasdf"
	pImpl.cfg.Metrics = data.Metrics{
		Mode:   "ON",
		Addr:   ":8081",
		Prefix: prefixOverride,
	}
	pImpl.Init()
	time.Sleep(1 * time.Second)
	resp, err := getMetrics()
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, pImpl.cfg.Metrics.Prefix, prefixOverride)
}

func TestPipelineTelemetryConfigs(t *testing.T) {
	pImpl, _, _, _, _ := mockPipeline(t, "")

	// telemetry OFF, check that client is nil
	pImpl.cfg.Telemetry = data.Telemetry{
		Enabled: false,
	}
	pImpl.Init()
	baseClient := (*pImpl.initProvider).GetTelemetryClient()
	assert.Nil(t, baseClient)

	// telemetry ON
	pImpl.cfg.Telemetry = data.Telemetry{
		Enabled:  true,
		URI:      "test-uri",
		Index:    "test-index",
		UserName: "test-username",
		Password: "test-password",
	}
	pImpl.Init()
	baseClient = (*pImpl.initProvider).GetTelemetryClient()
	client := baseClient.(*telemetry.OpenSearchClient)

	assert.NotNil(t, client)
	assert.NotNil(t, client.Client)
	assert.Equal(t, true, client.TelemetryConfig.Enable)
	assert.Equal(t, "test-uri", client.TelemetryConfig.URI)
	assert.Equal(t, "test-index", client.TelemetryConfig.Index)
	assert.Equal(t, "test-username", client.TelemetryConfig.UserName)
	assert.Equal(t, "test-password", client.TelemetryConfig.Password)

	event := client.MakeTelemetryStartupEvent()
	assert.Equal(t, "starting conduit", event.Message)
	assert.NotEmpty(t, event.Time)
	assert.NotEmpty(t, event.GUID)
	assert.NotEmpty(t, event.Version)
}

func TestRoundOverrideValidConflict(t *testing.T) {
	t.Run("processor_no_conflict", func(t *testing.T) {
		pImpl, _, mImporter, mProcessor, _ := mockPipeline(t, "")
		mImporter.rndOverride = 10
		mProcessor.rndOverride = 10
		err := pImpl.Init()
		assert.NoError(t, err)
	})

	t.Run("exporter_no_conflict", func(t *testing.T) {
		pImpl, _, _, mProcessor, mExporter := mockPipeline(t, "")
		mProcessor.rndOverride = 10
		mExporter.rndOverride = 10
		err := pImpl.Init()
		assert.NoError(t, err)
	})

	t.Run("cli_no_conflict", func(t *testing.T) {
		pImpl, _, mImporter, _, _ := mockPipeline(t, "")
		mImporter.rndOverride = 10
		pImpl.cfg.ConduitArgs.NextRoundOverride = 10
		err := pImpl.Init()
		assert.NoError(t, err)
	})
}

func TestRoundOverrideInvalidConflict(t *testing.T) {
	t.Run("processor_no_conflict", func(t *testing.T) {
		t.Parallel()
		pImpl, _, mImporter, mProcessor, _ := mockPipeline(t, "")
		mImporter.rndOverride = 1
		mProcessor.rndOverride = 10
		err := pImpl.Init()
		assert.ErrorIs(t, err, makeErrOverrideConflict("mockImporter", 1, "mockProcessor", 10))
	})

	t.Run("exporter_no_conflict", func(t *testing.T) {
		t.Parallel()
		pImpl, _, mImporter, _, mExporter := mockPipeline(t, "")
		mImporter.rndOverride = 1
		mExporter.rndOverride = 10
		err := pImpl.Init()
		assert.ErrorIs(t, err, makeErrOverrideConflict("mockImporter", 1, "mockExporter", 10))
	})

	t.Run("cli_no_conflict", func(t *testing.T) {
		t.Parallel()
		pImpl, _, mImporter, _, _ := mockPipeline(t, "")
		mImporter.rndOverride = 1
		pImpl.cfg.ConduitArgs.NextRoundOverride = 10
		err := pImpl.Init()
		assert.ErrorIs(t, err, makeErrOverrideConflict("mockImporter", 1, "command line", 10))
	})
}

func TestRoundRequestError(t *testing.T) {
	t.Run("importer round request error", func(t *testing.T) {
		t.Parallel()
		pImpl, _, mImporter, _, _ := mockPipeline(t, "")
		sentinelErr := errors.New("the error 1")
		mImporter.rndReqErr = sentinelErr
		err := pImpl.Init()
		assert.ErrorIs(t, err, sentinelErr)
	})

	t.Run("processor round request error", func(t *testing.T) {
		t.Parallel()
		pImpl, _, _, mProcessor, _ := mockPipeline(t, "")
		sentinelErr := errors.New("the error 2")
		mProcessor.rndReqErr = sentinelErr
		err := pImpl.Init()
		assert.ErrorIs(t, err, sentinelErr)
	})

	t.Run("exporter round request error", func(t *testing.T) {
		t.Parallel()
		pImpl, _, _, _, mExporter := mockPipeline(t, "")
		sentinelErr := errors.New("the error 3")
		mExporter.rndReqErr = sentinelErr
		err := pImpl.Init()
		assert.ErrorIs(t, err, sentinelErr)
	})
}

func TestRoundOverride(t *testing.T) {
	// cli override NextRound, 0 is a test for no override.
	for i := 0; i < 10; i++ {
		i := i
		t.Run(fmt.Sprintf("cli round override %d", i), func(t *testing.T) {
			t.Parallel()
			pImpl, _, _, _, _ := mockPipeline(t, "")
			pImpl.cfg.ConduitArgs.NextRoundOverride = uint64(i)
			err := pImpl.Init()
			assert.Nil(t, err)
			assert.Equal(t, uint64(i), pImpl.pipelineMetadata.NextRound)
		})
	}

	t.Run("importer round override", func(t *testing.T) {
		t.Parallel()
		pImpl, _, mImporter, _, _ := mockPipeline(t, "")
		mImporter.rndOverride = 10
		err := pImpl.Init()
		assert.Nil(t, err)
		assert.Equal(t, uint64(10), pImpl.pipelineMetadata.NextRound)
	})

	t.Run("processor round override", func(t *testing.T) {
		t.Parallel()
		pImpl, _, _, mProcessor, _ := mockPipeline(t, "")
		mProcessor.rndOverride = 10
		err := pImpl.Init()
		assert.Nil(t, err)
		assert.Equal(t, uint64(10), pImpl.pipelineMetadata.NextRound)
	})

	t.Run("exporter round override", func(t *testing.T) {
		t.Parallel()
		pImpl, _, _, _, mExporter := mockPipeline(t, "")
		mExporter.rndOverride = 10
		err := pImpl.Init()
		assert.Nil(t, err)
		assert.Equal(t, uint64(10), pImpl.pipelineMetadata.NextRound)
	})
}

// an importer that simply errors out when GetBlock() is called
type errorImporter struct {
	genesis       *sdk.Genesis
	GetBlockCount uint64
}

var errorImporterMetadata = plugins.Metadata{
	Name:         "error_importer",
	Description:  "An importer that errors out whenever GetBlock() is called",
	Deprecated:   false,
	SampleConfig: "",
}

func (e *errorImporter) Metadata() plugins.Metadata {
	return errorImporterMetadata
}

func (e *errorImporter) Init(_ context.Context, _ data.InitProvider, _ plugins.PluginConfig, _ *log.Logger) error {
	return nil
}

func (e *errorImporter) GetGenesis() (*sdk.Genesis, error) {
	return e.genesis, nil
}

func (e *errorImporter) Close() error {
	return nil
}

func (e *errorImporter) GetBlock(_ uint64) (data.BlockData, error) {
	e.GetBlockCount++
	return data.BlockData{}, fmt.Errorf("error maker")
}

// TestPipelineRetryVariables tests that modifying the retry variables results in longer time taken for a pipeline to run
func TestPipelineRetryVariables(t *testing.T) {
	maxDuration := 5 * time.Second
	epsilon := 250 * time.Millisecond // allow for some error in timing
	tests := []struct {
		name          string
		retryDelay    time.Duration
		retryCount    uint64
		totalDuration time.Duration
	}{
		{
			name:          "retryCount=0 (unlimited)",
			retryDelay:    500 * time.Millisecond,
			totalDuration: maxDuration,
		},
		{
			name:          "retryCount=1",
			retryDelay:    500 * time.Millisecond,
			retryCount:    1,
			totalDuration: 500 * time.Millisecond,
		},
		{
			name:          "retryCount=2",
			retryDelay:    500 * time.Millisecond,
			retryCount:    2,
			totalDuration: 1 * time.Second,
		},
		{
			name:          "retryCount=5",
			retryDelay:    500 * time.Millisecond,
			retryCount:    5,
			totalDuration: 2500 * time.Millisecond,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {

			errImporter := &errorImporter{genesis: &sdk.Genesis{Network: "test"}}
			var pImporter importers.Importer = errImporter
			var pProcessor processors.Processor = &mockProcessor{}
			var pExporter exporters.Exporter = &mockExporter{}
			l, hook := test.NewNullLogger()
			ctx, cf := context.WithCancel(context.Background())
			pImpl := pipelineImpl{
				ctx: ctx,
				cfg: &data.Config{
					RetryCount: testCase.retryCount,
					RetryDelay: testCase.retryDelay,
					ConduitArgs: &data.Args{
						ConduitDataDir:    t.TempDir(),
						NextRoundOverride: 0,
					},
					Importer: data.NameConfigPair{
						Name:   "",
						Config: map[string]interface{}{},
					},
					Processors: []data.NameConfigPair{
						{
							Name:   "",
							Config: map[string]interface{}{},
						},
					},
					Exporter: data.NameConfigPair{
						Name:   "unknown",
						Config: map[string]interface{}{},
					},
				},
				logger:       l,
				initProvider: nil,
				importer:     &pImporter,
				processors:   []*processors.Processor{&pProcessor},
				exporter:     &pExporter,
				pipelineMetadata: state{
					GenesisHash: "",
					Network:     "",
					NextRound:   3,
				},
				wg: sync.WaitGroup{},
			}

			err := pImpl.Init()
			assert.Nil(t, err)
			before := time.Now()
			done := false
			// test for "unlimited" timeout
			go func() {
				time.Sleep(maxDuration)
				if !done {
					cf()
					assert.Equal(t, testCase.totalDuration, maxDuration)
				}
			}()
			pImpl.Start()
			pImpl.wg.Wait()
			after := time.Now()
			timeTaken := after.Sub(before)

			msg := fmt.Sprintf("seconds taken: %s, expected duration seconds: %s, epsilon: %s", timeTaken.String(), testCase.totalDuration.String(), epsilon)
			assert.WithinDurationf(t, before.Add(testCase.totalDuration), after, epsilon, msg)
			if testCase.retryCount == 0 {
				assert.GreaterOrEqual(t, errImporter.GetBlockCount, uint64(1))
			} else {
				assert.Equal(t, errImporter.GetBlockCount, testCase.retryCount+1)
			}
			done = true
			fmt.Println(hook.AllEntries())
			for _, entry := range hook.AllEntries() {
				str, err := entry.String()
				require.NoError(t, err)
				if strings.HasPrefix(str, "Retry number 1") {
					assert.Equal(t, "Retry number 1 resuming after a 500ms retry delay.", str)
				}
			}
		})
	}
}

func TestMetricPrefixApplied(t *testing.T) {
	// Note: the default prefix is applied during `Init`, so no need to test that here.
	prefix := "test_prefix"
	tempDir := t.TempDir()
	pImpl, _, mImporter, _, _ := mockPipeline(t, tempDir)
	pImpl.cfg.Metrics.Prefix = prefix
	pImpl.registerPluginMetricsCallbacks()
	assert.Equal(t, prefix, mImporter.subsystem)
}

func TestMetrics(t *testing.T) {
	// This test cannot run in parallel because the metrics are global.
	basicTxn := func(t sdk.TxType) sdk.SignedTxnWithAD {
		return sdk.SignedTxnWithAD{
			SignedTxn: sdk.SignedTxn{
				Txn: sdk.Transaction{
					Type: t,
				},
			},
		}
	}
	txnWithInner := func(t sdk.TxType, inner ...sdk.SignedTxnWithAD) sdk.SignedTxnWithAD {
		result := basicTxn(t)
		result.EvalDelta.InnerTxns = inner
		return result
	}
	const round = sdk.Round(1234)

	block := data.BlockData{
		BlockHeader: sdk.BlockHeader{Round: round},
		Payset: []sdk.SignedTxnInBlock{
			{
				SignedTxnWithAD: basicTxn(sdk.PaymentTx),
			}, {
				SignedTxnWithAD: basicTxn(sdk.KeyRegistrationTx),
			}, {
				SignedTxnWithAD: basicTxn(sdk.AssetConfigTx),
			}, {
				SignedTxnWithAD: basicTxn(sdk.AssetTransferTx),
			}, {
				SignedTxnWithAD: basicTxn(sdk.AssetFreezeTx),
			}, {
				SignedTxnWithAD: basicTxn(sdk.ApplicationCallTx),
			}, {
				SignedTxnWithAD: basicTxn(sdk.StateProofTx),
			}, {
				// counted as 1 app call and 6 inner txns
				SignedTxnWithAD: txnWithInner(sdk.ApplicationCallTx,
					basicTxn(sdk.PaymentTx),
					txnWithInner(sdk.ApplicationCallTx,
						basicTxn(sdk.PaymentTx),
						basicTxn(sdk.PaymentTx)),
					basicTxn(sdk.PaymentTx),
					basicTxn(sdk.PaymentTx)),
			},
		},
	}

	assert.Equal(t, 6, numInnerTxn(block.Payset[7].SignedTxnWithAD))

	metrics.RegisterPrometheusMetrics("add_metrics_test")
	addMetrics(block, time.Hour)
	stats, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	found := 0
	for _, stat := range stats {
		if strings.HasSuffix(*stat.Name, metrics.BlockImportTimeName) {
			found++
			// 1 hour in seconds
			assert.Contains(t, stat.String(), "sample_count:1 sample_sum:3600")
		}
		if strings.HasSuffix(*stat.Name, metrics.ImportedRoundGaugeName) {
			found++
			assert.Contains(t, stat.String(), "value:1234")
		}
		if strings.HasSuffix(*stat.Name, metrics.ImportedTxnsPerBlockName) {
			found++
			assert.Contains(t, stat.String(), "sample_count:1 sample_sum:14")
		}
		if strings.HasSuffix(*stat.Name, metrics.ImportedTxnsName) {
			found++
			str := stat.String()
			// the 6 single txns
			assert.Contains(t, str, `label:<name:"txn_type" value:"acfg" > gauge:<value:1 >`)
			assert.Contains(t, str, `label:<name:"txn_type" value:"afrz" > gauge:<value:1 >`)
			assert.Contains(t, str, `label:<name:"txn_type" value:"axfer" > gauge:<value:1 >`)
			assert.Contains(t, str, `label:<name:"txn_type" value:"keyreg" > gauge:<value:1 >`)
			assert.Contains(t, str, `label:<name:"txn_type" value:"pay" > gauge:<value:1 >`)
			assert.Contains(t, str, `label:<name:"txn_type" value:"stpf" > gauge:<value:1 >`)

			// 2 app call txns
			assert.Contains(t, str, `label:<name:"txn_type" value:"appl" > gauge:<value:2 >`)

			// 1 app had 6 inner txns
			assert.Contains(t, str, `label:<name:"txn_type" value:"inner" > gauge:<value:6 >`)
		}
	}
	assert.Equal(t, 4, found)
}
