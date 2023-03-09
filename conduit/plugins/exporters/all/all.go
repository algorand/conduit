package all

import (
	// Call package wide init function
	_ "github.com/algorand/conduit/conduit/plugins/exporters/filewriter"
	_ "github.com/algorand/conduit/conduit/plugins/exporters/noop"
	_ "github.com/algorand/conduit/conduit/plugins/exporters/postgresql"
)
