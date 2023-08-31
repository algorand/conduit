package noop

import (
	"context"
	_ "embed" // used to embed config
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/importers"
)

// PluginName to use when configuring.
var PluginName = "noop"

const sleepForGetBlock = 100 * time.Millisecond

// `noopImporter`s will function without ever erroring. This means they will also process out of order blocks
// which may or may not be desirable for different use cases--it can hide errors in actual importers expecting in order
// block processing.
// The `noopImporter` will maintain `Round` state according to the round of the last block it processed.
// It also sleeps 100 milliseconds between blocks to slow down the pipeline.
type noopImporter struct {
	round uint64
	cfg   ImporterConfig
}

//go:embed sample.yaml
var sampleConfig string

var metadata = plugins.Metadata{
	Name:         PluginName,
	Description:  "noop importer",
	Deprecated:   false,
	SampleConfig: sampleConfig,
}

func (imp *noopImporter) Metadata() plugins.Metadata {
	return metadata
}

func (imp *noopImporter) Init(_ context.Context, _ data.InitProvider, cfg plugins.PluginConfig, _ *logrus.Logger) error {
	if err := cfg.UnmarshalConfig(&imp.cfg); err != nil {
		return fmt.Errorf("init failure in unmarshalConfig: %v", err)
	}
	imp.round = imp.cfg.Round
	return nil
}

func (imp *noopImporter) Close() error {
	return nil
}

func (imp *noopImporter) GetGenesis() (*sdk.Genesis, error) {
	return &sdk.Genesis{}, nil
}

func (imp *noopImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	time.Sleep(sleepForGetBlock)
	imp.round = rnd
	return data.BlockData{
		BlockHeader: sdk.BlockHeader{
			Round: sdk.Round(rnd),
		},
	}, nil
}

func (imp *noopImporter) Round() uint64 {
	return imp.round
}

func init() {
	importers.Register(PluginName, importers.ImporterConstructorFunc(func() importers.Importer {
		return &noopImporter{}
	}))
}
