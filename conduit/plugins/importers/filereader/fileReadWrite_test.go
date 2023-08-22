package fileimporter

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	logrusTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters"
	"github.com/algorand/conduit/conduit/plugins/exporters/filewriter"
	"github.com/algorand/conduit/conduit/plugins/importers"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

const (
	conduitDataDir   = "test_resources/conduit_data"
	filePattern      = "%[1]d_block.msgp.gz"
	importerBlockDir = "test_resources/filereader_blocks"
	exporterBlockDir = "test_resources/conduit_data/exporter_file_writer"
)

func cleanArtifacts(t *testing.T) {
	err := os.RemoveAll(exporterBlockDir)
	require.NoError(t, err)
}

// numGzippedFiles returns the number of files in the importerBlockDir
// whose filename ends in .gz
func numGzippedFiles(t *testing.T) uint64 {
	files, err := os.ReadDir(importerBlockDir)
	require.NoError(t, err)

	gzCount := uint64(0)
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".gz") {
			gzCount++
		}
	}

	return gzCount
}

func fileBytes(t *testing.T, path string) []byte {
	file, err := os.Open(path)
	require.NoError(t, err, "error opening file %s", path)
	defer file.Close()

	var buf bytes.Buffer
	_, err = buf.ReadFrom(file)
	require.NoError(t, err, "error reading file %s", path)

	return buf.Bytes()
}

func identicalFiles(t *testing.T, path1, path2 string) {
	var file1, file2 *os.File

	defer func() {
		if file1 != nil {
			file1.Close()
		}
		if file2 != nil {
			file2.Close()
		}
	}()

	bytes1 := fileBytes(t, path1)
	bytes2 := fileBytes(t, path2)
	require.Equal(t, len(bytes1), len(bytes2), "files %s and %s have different lengths", path1, path2)

	for i, b1 := range bytes1 {
		b2 := bytes2[i]
		require.Equal(t, b1, b2, "files %s and %s differ at byte %d (%s) v (%s)", path1, path2, i, string(b1), string(b2))
	}
}

func uncompressBytes(t *testing.T, path string) []byte {
	file, err := os.Open(path)
	require.NoError(t, err, "error opening file %s", path)
	defer file.Close()

	gr, err := gzip.NewReader(file)
	require.NoError(t, err, "error creating gzip reader for file %s", path)
	defer gr.Close()

	var buf bytes.Buffer
	_, err = buf.ReadFrom(gr)
	require.NoError(t, err, "error reading file %s", path)

	return buf.Bytes()
}
func identicalFilesUncompressed(t *testing.T, path1, path2 string) {
	var file1, file2 *os.File

	defer func() {
		if file1 != nil {
			file1.Close()
		}
		if file2 != nil {
			file2.Close()
		}
	}()

	bytes1 := uncompressBytes(t, path1)
	bytes2 := uncompressBytes(t, path2)
	require.Equal(t, len(bytes1), len(bytes2), "files %s and %s have different lengths", path1, path2)

	for i, b1 := range bytes1 {
		b2 := bytes2[i]
		require.Equal(t, b1, b2, "files %s and %s differ at byte %d (%s) v (%s)", path1, path2, i, string(b1), string(b2))
	}
}

func getConfig(t *testing.T, pt plugins.PluginType, cfg data.NameConfigPair, dataDir string) plugins.PluginConfig {
	configs, err := yaml.Marshal(cfg.Config)
	require.NoError(t, err)

	var config plugins.PluginConfig
	config.Config = string(configs)
	if dataDir != "" {
		config.DataDir = path.Join(dataDir, fmt.Sprintf("%s_%s", pt, cfg.Name))
		err = os.MkdirAll(config.DataDir, os.ModePerm)
		require.NoError(t, err)
	}

	return config
}

// TestRoundTrip tests that blocks read by the filereader importer
// under the msgp.gz encoding are written to identical files by the filewriter exporter.
// This includes both a genesis block and a round-0 block with different encodings.
func TestRoundTrip(t *testing.T) {
	cleanArtifacts(t)
	defer cleanArtifacts(t)

	round := sdk.Round(0)
	lastRound := numGzippedFiles(t) - 2 // subtract round-0 and the separate genesis file
	require.GreaterOrEqual(t, lastRound, uint64(1))
	require.LessOrEqual(t, lastRound, uint64(1000)) // overflow sanity check

	ctx := context.Background()

	plineConfig, err := data.MakePipelineConfig(&data.Args{
		ConduitDataDir: conduitDataDir,
	})
	require.NoError(t, err)

	logger, _ := logrusTest.NewNullLogger()

	// Assert configurations:
	require.Equal(t, "file_reader", plineConfig.Importer.Name)
	require.Equal(t, importerBlockDir, plineConfig.Importer.Config["block-dir"])
	require.Equal(t, filePattern, plineConfig.Importer.Config["filename-pattern"])

	require.Equal(t, "file_writer", plineConfig.Exporter.Name)
	require.Equal(t, filePattern, plineConfig.Exporter.Config["filename-pattern"])
	require.False(t, plineConfig.Exporter.Config["drop-certificate"].(bool))

	// Simulate the portions of the pipeline's Init() that interact
	// with the importer and exporter
	initProvider := conduit.MakePipelineInitProvider(&round, nil, nil)

	// Importer init
	impCtor, err := importers.ImporterConstructorByName(plineConfig.Importer.Name)
	require.NoError(t, err)
	importer := impCtor.New()
	impConfig := getConfig(t, plugins.Importer, plineConfig.Importer, conduitDataDir)
	require.NoError(t, err)
	require.Equal(t, path.Join(conduitDataDir, "importer_file_reader"), impConfig.DataDir)

	err = importer.Init(ctx, initProvider, impConfig, logger)
	require.NoError(t, err)

	impGenesis, err := importer.GetGenesis()
	require.NoError(t, err)
	require.Equal(t, "generated-network", impGenesis.Network)

	genesisFile := filewriter.GenesisFilename
	// it should be the same as unmarshalling it directly from the expected path
	require.Equal(t, "genesis.json", genesisFile)
	require.NoError(t, err)

	impGenesisPath := path.Join(importerBlockDir, genesisFile)
	genesis := &sdk.Genesis{}

	err = filewriter.DecodeFromFile(impGenesisPath, genesis, filewriter.JSONFormat, false)
	require.NoError(t, err)

	require.Equal(t, impGenesis, genesis)

	initProvider.SetGenesis(impGenesis)

	// Construct the exporter
	expCtor, err := exporters.ExporterConstructorByName(plineConfig.Exporter.Name)
	require.NoError(t, err)
	exporter := expCtor.New()
	expConfig := getConfig(t, plugins.Exporter, plineConfig.Exporter, conduitDataDir)
	require.NoError(t, err)
	require.Equal(t, path.Join(conduitDataDir, "exporter_file_writer"), expConfig.DataDir)

	err = exporter.Init(ctx, initProvider, expConfig, logger)
	require.NoError(t, err)

	// It should have persisted the genesis which ought to be identical
	// to the importer's.
	expGenesisPath := path.Join(exporterBlockDir, genesisFile)
	identicalFiles(t, impGenesisPath, expGenesisPath)

	// Simulate the pipeline
	require.Equal(t, sdk.Round(0), round)
	for ; uint64(round) <= lastRound; round++ {
		blk, err := importer.GetBlock(uint64(round))
		require.NoError(t, err)

		expBlockPath := path.Join(exporterBlockDir, fmt.Sprintf(filePattern, round))
		_, err = os.OpenFile(expBlockPath, os.O_RDONLY, 0)
		require.ErrorIs(t, err, os.ErrNotExist)

		err = exporter.Receive(blk)
		require.NoError(t, err)

		_, err = os.OpenFile(expBlockPath, os.O_RDONLY, 0)
		require.NoError(t, err)

		impBlockBath := path.Join(importerBlockDir, fmt.Sprintf(filePattern, round))

		identicalFilesUncompressed(t, impBlockBath, expBlockPath)
	}
}
