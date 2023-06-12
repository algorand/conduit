package importers

import (
	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
)

// Importer defines the interface for importer plugins
type Importer interface {
	// Plugin - implement this interface.
	plugins.Plugin

	// GetGenesis returns the genesis object for the network.
	// It may only be called after Init().
	GetGenesis() (*sdk.Genesis, error)

	// GetBlock given any round number-rnd fetches the block at that round.
	// It returns an object of type BlockData defined in data.
	GetBlock(rnd uint64) (data.BlockData, error)
}
