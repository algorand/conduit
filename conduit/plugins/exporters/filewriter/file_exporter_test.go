package filewriter

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

const (
	defaultEncodingFormat = MessagepackFormat
	defaultIsGzip         = true
)

var logger *logrus.Logger
var fileCons = exporters.ExporterConstructorFunc(func() exporters.Exporter {
	return &fileExporter{}
})
var configTemplatePrefix = "block-dir: %s/blocks\n"
var round = sdk.Round(2)

func init() {
	logger, _ = test.NewNullLogger()
}

func getConfigWithoutPattern(t *testing.T) (config, tempdir string) {
	tempdir = t.TempDir()
	config = fmt.Sprintf(configTemplatePrefix, tempdir)
	return
}

func getConfigWithPattern(t *testing.T, pattern string) (config, tempdir string) {
	config, tempdir = getConfigWithoutPattern(t)
	config = fmt.Sprintf("%sfilename-pattern: '%s'\n", config, pattern)
	return
}

func TestDefaults(t *testing.T) {
	require.Equal(t, defaultEncodingFormat, MessagepackFormat)
	require.Equal(t, defaultIsGzip, true)
}

func TestExporterMetadata(t *testing.T) {
	fileExp := fileCons.New()
	meta := fileExp.Metadata()
	require.Equal(t, metadata.Name, meta.Name)
	require.Equal(t, metadata.Description, meta.Description)
	require.Equal(t, metadata.Deprecated, meta.Deprecated)
}

func TestExporterInitDefaults(t *testing.T) {
	tempdir := t.TempDir()
	override := path.Join(tempdir, "override")

	testcases := []struct {
		blockdir string
		expected string
	}{
		{
			blockdir: "",
			expected: tempdir,
		},
		{
			blockdir: "''",
			expected: tempdir,
		},
		{
			blockdir: override,
			expected: override,
		},
		{
			blockdir: fmt.Sprintf("'%s'", override),
			expected: override,
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(fmt.Sprintf("blockdir=%s", tc.blockdir), func(t *testing.T) {
			t.Parallel()
			fileExp := fileCons.New()
			defer fileExp.Close()
			pcfg := plugins.MakePluginConfig(fmt.Sprintf("block-dir: %s", tc.blockdir))
			pcfg.DataDir = tempdir
			err := fileExp.Init(context.Background(), conduit.MakePipelineInitProvider(&round, nil, nil), pcfg, logger)
			require.NoError(t, err)
		})
	}
}

func TestExporterInit(t *testing.T) {
	config, _ := getConfigWithPattern(t, "%[1]d_block.json")
	fileExp := fileCons.New()
	defer fileExp.Close()

	// creates a new output file
	err := fileExp.Init(context.Background(), conduit.MakePipelineInitProvider(&round, nil, nil), plugins.MakePluginConfig(config), logger)
	require.NoError(t, err)
	fileExp.Close()

	// can open existing file
	err = fileExp.Init(context.Background(), conduit.MakePipelineInitProvider(&round, nil, nil), plugins.MakePluginConfig(config), logger)
	require.NoError(t, err)
	fileExp.Close()
}

func sendData(t *testing.T, fileExp exporters.Exporter, config string, numRounds int) {
	// Test invalid block receive
	block := data.BlockData{
		BlockHeader: sdk.BlockHeader{
			Round: 3,
			StateProofTracking: map[sdk.StateProofType]sdk.StateProofTrackingData{
				0: {StateProofNextRound: 2},
			},
		},
		Payset:      nil,
		Delta:       nil,
		Certificate: nil,
	}

	err := fileExp.Receive(block)
	require.Contains(t, err.Error(), "exporter not initialized")

	// initialize
	rnd := sdk.Round(0)
	err = fileExp.Init(context.Background(), conduit.MakePipelineInitProvider(&rnd, nil, nil), plugins.MakePluginConfig(config), logger)
	require.NoError(t, err)

	// incorrect round
	err = fileExp.Receive(block)
	require.Contains(t, err.Error(), "received round 3, expected round 0")

	// write block to file
	for i := sdk.Round(0); int(i) < numRounds; i++ {
		block = data.BlockData{
			BlockHeader: sdk.BlockHeader{
				Round: i,
				StateProofTracking: map[sdk.StateProofType]sdk.StateProofTrackingData{
					0: {StateProofNextRound: i},
				},
			},
			Payset: nil,
			Delta: &sdk.LedgerStateDelta{
				PrevTimestamp: 1234,
			},
			Certificate: &map[string]interface{}{
				"Round":  i,
				"Period": 2,
				"Step":   2,
			},
		}
		err = fileExp.Receive(block)
		require.NoError(t, err)
	}

	require.NoError(t, fileExp.Close())
}

func TestExporterReceive(t *testing.T) {
	patterns := []string{
		"%[1]d_block.json",
		"%[1]d_block.json.gz",
		"%[1]d_block.msgp",
		"%[1]d_block.msgp.gz",
	}
	for _, pattern := range patterns {
		pattern := pattern
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()

			format, isGzip, err := ParseFilenamePattern(pattern)
			require.NoError(t, err)
			config, tempdir := getConfigWithPattern(t, pattern)
			fileExp := fileCons.New()
			numRounds := 5
			sendData(t, fileExp, config, numRounds)

			// block data is valid
			for i := 0; i < 5; i++ {
				filename := fmt.Sprintf(pattern, i)
				path := fmt.Sprintf("%s/blocks/%s", tempdir, filename)
				require.FileExists(t, path)

				blockBytes, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NotContains(t, string(blockBytes), " 0: ")

				var blockData data.BlockData
				err = DecodeFromFile(path, &blockData, format, isGzip)
				require.NoError(t, err)
				require.Equal(t, sdk.Round(i), blockData.BlockHeader.Round)
				require.NotNil(t, blockData.Certificate)
			}
		})
	}
}

func TestExporterClose(t *testing.T) {
	config, _ := getConfigWithoutPattern(t)
	fileExp := fileCons.New()
	rnd := sdk.Round(0)
	fileExp.Init(context.Background(), conduit.MakePipelineInitProvider(&rnd, nil, nil), plugins.MakePluginConfig(config), logger)
	require.NoError(t, fileExp.Close())
}

func TestPatternDefault(t *testing.T) {
	config, tempdir := getConfigWithoutPattern(t)
	fileExp := fileCons.New()

	numRounds := 5
	sendData(t, fileExp, config, numRounds)

	// block data is valid
	for i := 0; i < 5; i++ {
		filename := fmt.Sprintf(FilePattern, i)
		path := fmt.Sprintf("%s/blocks/%s", tempdir, filename)
		require.FileExists(t, path)

		var blockData data.BlockData
		err := DecodeFromFile(path, &blockData, defaultEncodingFormat, defaultIsGzip)
		require.Equal(t, sdk.Round(i), blockData.BlockHeader.Round)
		require.NoError(t, err)
		require.NotNil(t, blockData.Certificate)
	}
}

func TestDropCertificate(t *testing.T) {
	tempdir := t.TempDir()
	cfg := Config{
		BlocksDir:       tempdir,
		DropCertificate: true,
	}
	config, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	numRounds := 10
	exporter := fileCons.New()
	sendData(t, exporter, string(config), numRounds)

	// block data is valid
	for i := 0; i < numRounds; i++ {
		filename := fmt.Sprintf(FilePattern, i)
		path := fmt.Sprintf("%s/%s", tempdir, filename)
		require.FileExists(t, path)
		var blockData data.BlockData
		err := DecodeFromFile(path, &blockData, defaultEncodingFormat, defaultIsGzip)
		require.NoError(t, err)
		require.Nil(t, blockData.Certificate)
	}
}
