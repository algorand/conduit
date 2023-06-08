package exporters

import (
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
)

// Exporter defines the interface for plugins
type Exporter interface {
	// Plugin - implement this interface.
	plugins.Plugin

	// Receive is called for each block to be processed by the exporter.
	// Should return an error on failure--retries are configurable.
	Receive(exportData data.BlockData) error
}
