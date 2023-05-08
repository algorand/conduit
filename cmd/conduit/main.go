package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra/doc"

	_ "github.com/algorand/conduit/conduit/plugins/exporters/all"
	_ "github.com/algorand/conduit/conduit/plugins/importers/all"
	_ "github.com/algorand/conduit/conduit/plugins/processors/all"
	"github.com/algorand/conduit/pkg/cli"
)

func main() {
	conduitCmd := cli.MakeConduitCmdWithUtilities()

	// Hidden command to generate docs in a given directory
	// conduit generate-docs [path]
	if len(os.Args) == 3 && os.Args[1] == "generate-docs" {
		err := doc.GenMarkdownTree(conduitCmd, os.Args[2])
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := conduitCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}
