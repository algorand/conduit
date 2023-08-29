package fileimporter

import (
	"context"
	_ "embed" // used to embed config
	"fmt"
	"path"
	"time"

	"github.com/sirupsen/logrus"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters/filewriter"
	"github.com/algorand/conduit/conduit/plugins/importers"
)

// PluginName to use when configuring.
const PluginName = "file_reader"

type fileReader struct {
	logger *logrus.Logger
	cfg    Config
	gzip   bool
	format filewriter.EncodingFormat
	ctx    context.Context
	cancel context.CancelFunc
}

// package-wide init function
func init() {
	importers.Register(PluginName, importers.ImporterConstructorFunc(func() importers.Importer {
		return &fileReader{}
	}))
}

// New initializes an algod importer
func New() importers.Importer {
	return &fileReader{}
}

//go:embed sample.yaml
var sampleConfig string

var metadata = plugins.Metadata{
	Name:         PluginName,
	Description:  "Importer for fetching blocks from files in a directory created by the 'file_writer' plugin.",
	Deprecated:   false,
	SampleConfig: sampleConfig,
}

func (r *fileReader) Metadata() plugins.Metadata {
	return metadata
}

func (r *fileReader) Init(ctx context.Context, _ data.InitProvider, cfg plugins.PluginConfig, logger *logrus.Logger) error {
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.logger = logger
	err := cfg.UnmarshalConfig(&r.cfg)
	if err != nil {
		return fmt.Errorf("invalid configuration: %v", err)
	}

	if r.cfg.FilenamePattern == "" {
		r.cfg.FilenamePattern = filewriter.FilePattern
	}
	r.format, r.gzip, err = filewriter.ParseFilenamePattern(r.cfg.FilenamePattern)
	if err != nil {
		return fmt.Errorf("Init() error: %w", err)
	}

	return nil
}

// GetGenesis returns the genesis. Is is assumed that the genesis file is available as `genesis.json`
// regardless of chosen encoding format and gzip flag.
// It is also assumed that there is a separate round 0 block file adhering to the expected filename pattern with encoding.
// This is because genesis and round 0 block have different data and encodings,
// and the official network genesis files are plain uncompressed JSON.
func (r *fileReader) GetGenesis() (*sdk.Genesis, error) {
	var genesis sdk.Genesis
	err := filewriter.DecodeFromFile(path.Join(r.cfg.BlocksDir, filewriter.GenesisFilename), &genesis, filewriter.JSONFormat, false)
	if err != nil {
		return nil, fmt.Errorf("GetGenesis(): failed to process genesis file: %w", err)
	}
	return &genesis, nil
}

func (r *fileReader) Close() error {
	if r.cancel != nil {
		r.cancel()
	}
	return nil
}

func (r *fileReader) GetBlock(rnd uint64) (data.BlockData, error) {
	filename := path.Join(r.cfg.BlocksDir, fmt.Sprintf(r.cfg.FilenamePattern, rnd))
	var blockData data.BlockData
	start := time.Now()

	// Read file content
	err := filewriter.DecodeFromFile(filename, &blockData, r.format, r.gzip)
	if err != nil {
		return data.BlockData{}, fmt.Errorf("GetBlock(): unable to read block file '%s': %w", filename, err)
	}
	r.logger.Infof("Block %d read time: %s", rnd, time.Since(start))
	return blockData, nil
}
