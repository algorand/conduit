package fileimporter

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters/filewriter"
	"github.com/algorand/conduit/conduit/plugins/importers"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

const (
	defaultEncodingFormat = filewriter.MessagepackFormat
	defaultIsGzip         = true
)

var (
	logger       *logrus.Logger
	testImporter importers.Importer
	pRound       sdk.Round
)

func init() {
	logger = logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)
	pRound = sdk.Round(1)
}

func TestDefaults(t *testing.T) {
	format, gzip, err := filewriter.ParseFilenamePattern(filewriter.FilePattern)
	require.NoError(t, err)
	require.Equal(t, format, defaultEncodingFormat)
	require.Equal(t, gzip, defaultIsGzip)
}

func TestImporterorterMetadata(t *testing.T) {
	testImporter = New()
	m := testImporter.Metadata()
	assert.Equal(t, metadata.Name, m.Name)
	assert.Equal(t, metadata.Description, m.Description)
	assert.Equal(t, metadata.Deprecated, m.Deprecated)
}

// initializeTestData fills a data directory with some dummy data for the importer to read.
func initializeTestData(t *testing.T, dir string, numRounds int) sdk.Genesis {
	genesisA := sdk.Genesis{
		SchemaID:    "test",
		Network:     "test",
		Proto:       "test",
		Allocation:  nil,
		RewardsPool: "AAAAAAA",
		FeeSink:     "AAAAAAA",
		Timestamp:   1234,
	}

	err := filewriter.EncodeToFile(path.Join(dir, filewriter.GenesisFilename), genesisA, filewriter.JSONFormat, false)
	require.NoError(t, err)

	for i := 0; i < numRounds; i++ {
		block := data.BlockData{
			BlockHeader: sdk.BlockHeader{
				Round: sdk.Round(i),
			},
			Payset:      make([]sdk.SignedTxnInBlock, 0),
			Delta:       &sdk.LedgerStateDelta{},
			Certificate: nil,
		}
		blockFile := path.Join(dir, fmt.Sprintf(filewriter.FilePattern, i))
		err = filewriter.EncodeToFile(blockFile, block, defaultEncodingFormat, defaultIsGzip)
		require.NoError(t, err)
	}

	return genesisA
}

func initializeImporter(t *testing.T, numRounds int) (importer importers.Importer, tempdir string, genesis *sdk.Genesis, err error) {
	tempdir = t.TempDir()
	genesisExpected := initializeTestData(t, tempdir, numRounds)
	importer = New()
	cfg := Config{
		BlocksDir: tempdir,
	}
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	err = importer.Init(context.Background(), conduit.MakePipelineInitProvider(&pRound, nil, nil), plugins.MakePluginConfig(string(data)), logger)
	assert.NoError(t, err)
	genesis, err = importer.GetGenesis()
	require.NoError(t, err)
	require.NotNil(t, genesis)
	require.Equal(t, genesisExpected, *genesis)
	return
}

func TestInitSuccess(t *testing.T) {
	_, _, _, err := initializeImporter(t, 1)
	require.NoError(t, err)
}

func TestInitUnmarshalFailure(t *testing.T) {
	testImporter = New()
	err := testImporter.Init(context.Background(), conduit.MakePipelineInitProvider(&pRound, nil, nil), plugins.MakePluginConfig("`"), logger)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "invalid configuration")
	testImporter.Close()
}

func TestGetBlockSuccess(t *testing.T) {
	numRounds := 10
	importer, tempdir, genesis, err := initializeImporter(t, 10)
	require.NoError(t, err)
	require.NotEqual(t, "", tempdir)
	require.NotNil(t, genesis)

	for i := 0; i < numRounds; i++ {
		block, err := importer.GetBlock(uint64(i))
		require.NoError(t, err)
		require.Equal(t, sdk.Round(i), block.BlockHeader.Round)
	}
}
