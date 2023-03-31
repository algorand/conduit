package processors

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
)

// Processor an interface that defines an object that can filter and process transactions
type Processor interface {
	// PluginMetadata implement this interface.
	plugins.PluginMetadata

	// Config returns the configuration options used to create the Processor.
	Config() string

	// Init will be called during initialization, before block data starts going through the pipeline.
	// Typically, used for things like initializing network connections.
	// The Context passed to Init() will be used for deadlines, cancel signals and other early terminations
	// The Config passed to Init() will contain the unmarshalled config file specific to this plugin.
	Init(ctx context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig, logger *logrus.Logger) error

	// Close will be called during termination of the Indexer process.
	Close() error

	// Process will be called with provided optional inputs.  It is up to the plugin to check that required inputs are provided.
	Process(input data.BlockData) (data.BlockData, error)
}
