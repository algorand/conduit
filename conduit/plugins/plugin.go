package plugins

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/algorand/conduit/conduit/data"
)

// Plugin is the common interface for all Conduit plugins.
type Plugin interface {
	// Metadata associated with the plugin.
	Metadata() Metadata

	// Init will be called during initialization, before block data starts going through the pipeline.
	// Typically this is used for things like initializing network connections.
	// The Context passed to Init() will be used for deadlines, cancelling signals and other early terminations.
	// The InitProvider passed to Init() will be used to determine the initial next round for consideration.
	// The Config passed to Init() will contain the unmarshalled config file specific to this plugin.
	// If any of these fail an error is returned --this will result in the Conduit process terminating.
	Init(ctx context.Context, initProvider data.InitProvider, cfg PluginConfig, logger *logrus.Logger) error

	// Close will be called during termination of the Conduit process.
	// There is no guarantee that plugin lifecycle hooks will be invoked in any specific order in relation to one another.
	// Returns an error if it fails which will be surfaced in the logs, but the process is already terminating.
	Close() error
}
