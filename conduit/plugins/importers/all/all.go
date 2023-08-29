package all

import (
	// Call package wide init function
	_ "github.com/algorand/conduit/conduit/plugins/importers/algod"
	_ "github.com/algorand/conduit/conduit/plugins/importers/filereader"
	_ "github.com/algorand/conduit/conduit/plugins/importers/noop"
)
