package fileimporter

import (
	"context"
	_ "embed" // used to embed config
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
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
	gzip  bool
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

// GetGenesis returns the genesis. Is is assumed that
// the genesis file is available __in addition to__ the round 0 block file.
// This is because the encoding assumed for the genesis is different
// from the encoding assumed for blocks.
// TODO: handle the case of a multipurpose file that contains both encodings.
func (r *fileReader) GetGenesis() (*sdk.Genesis, error) {
	genesisFile, err := filewriter.GenesisFilename(r.format, r.gzip)
	if err != nil {
		return nil, fmt.Errorf("GetGenesis(): failed to get genesis filename: %w", err)
	}
	genesisFile = path.Join(r.cfg.BlocksDir, genesisFile)

	var genesis sdk.Genesis
	err = filewriter.DecodeFromFile(genesisFile, &genesis, r.format, r.gzip)
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

func posErr(file string, err error) error {
	pattern := `pos (\d+)`
	re := regexp.MustCompile(pattern)

	// Find the position
	match := re.FindStringSubmatch(err.Error())
	var position int
	if len(match) > 1 {
		var err2 error
		position, err2 = strconv.Atoi(match[1])
		if err2 != nil {
			return fmt.Errorf("unable to parse position: %w, err: %w", err2, err)
		}
	} else {
		return fmt.Errorf("unknown error: %w", err)
	}

	content, err2 := os.ReadFile(file)
	if err2 != nil {
		return fmt.Errorf("error reading file: %w, err: %w", err2, err)
	}

	radius := 20
	start := position - radius
	if start < 0 {
		start = 0
	}
	end := position + radius
	if end > len(content) {
		end = len(content)
	}

	return fmt.Errorf(`error in %s @position %d: %w
<<<<<%s>>>>>`, file, position, err, string(content[start:end]))
}

func (r *fileReader) GetBlock(rnd uint64) (data.BlockData, error) {
	filename := path.Join(r.cfg.BlocksDir, fmt.Sprintf(r.cfg.FilenamePattern, rnd))
	var blockData data.BlockData
	start := time.Now()

	// Read file content
	err := filewriter.DecodeFromFile(filename, &blockData, r.format, r.gzip)
	if err != nil {
		err = posErr(filename, err)
		return data.BlockData{}, fmt.Errorf("GetBlock(): unable to read block file '%s': %w", filename, err)
	}
	r.logger.Infof("Block %d read time: %s", rnd, time.Since(start))
	return blockData, nil
}
