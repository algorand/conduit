package processors

import (
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
)

// Processor an interface that defines an object that can filter and process transactions
type Processor interface {
	// Plugin - implement this interface.
	plugins.Plugin

	// Process will be called with provided optional inputs.
	// It is up to the plugin to check that required inputs are provided.
	Process(input data.BlockData) (data.BlockData, error)
}
