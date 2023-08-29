package noop

import (
	"context"
	_ "embed" // used to embed config
	"fmt"

	"github.com/sirupsen/logrus"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/importers"
)

// PluginName to use when configuring.
var PluginName = "noop"

// `noopImporter`s will function without ever erroring. This means they will also process out of order blocks
// which may or may not be desirable for different use cases--it can hide errors in actual importers expecting in order
// block processing.
// The `noopImporter` will maintain `Round` state according to the round of the last block it processed.
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

func (exp *noopImporter) Metadata() plugins.Metadata {
	return metadata
}

func (exp *noopImporter) Init(_ context.Context, _ data.InitProvider, cfg plugins.PluginConfig, _ *logrus.Logger) error {
	if err := cfg.UnmarshalConfig(&exp.cfg); err != nil {
		return fmt.Errorf("init failure in unmarshalConfig: %v", err)
	}
	exp.round = exp.cfg.Round
	return nil
}

func (exp *noopImporter) Close() error {
	return nil
}

func (i *noopImporter) GetGenesis() (*sdk.Genesis, error) {
	return &sdk.Genesis{}, nil
}

func (exp *noopImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	exp.round = rnd
	return data.BlockData{
		BlockHeader: sdk.BlockHeader{
			Round: sdk.Round(rnd),
		},
	}, nil
}

func (exp *noopImporter) Round() uint64 {
	return exp.round
}

func init() {
	importers.Register(PluginName, importers.ImporterConstructorFunc(func() importers.Importer {
		return &noopImporter{}
	}))
}
