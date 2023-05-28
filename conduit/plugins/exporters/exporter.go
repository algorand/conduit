package exporters

import (
	"context"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/sirupsen/logrus"
)

// Exporter defines the interface for plugins
type Exporter interface {
	// Plugin - implement this interface.
	plugins.Plugin

	// Init will be called during initialization, before block data starts going through the pipeline.
	// Typically used for things like initializing network connections.
	// The ExporterConfig passed to Init will contain the Unmarhsalled config file specific to this plugin.
	// Should return an error if it fails--this will result in the Conduit process terminating.
	Init(ctx context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig, logger *logrus.Logger) error

	// Receive is called for each block to be processed by the exporter.
	// Should return an error on failure--retries are configurable.
	Receive(exportData data.BlockData) error
}
