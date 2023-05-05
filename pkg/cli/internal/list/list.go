package list

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/algorand/conduit/conduit/pipeline"
	"github.com/algorand/conduit/conduit/plugins"
)

// Command is the list command to embed in a root cobra command.
var Command = &cobra.Command{
	Use:   "list",
	Short: "List all available Conduit plugins",
	Long: `List all available Conduit plugins by type and a short description.

Use this utility to explore the plugins. Drill into each plugin to get a
sample configuration.

Example:
  conduit list importers algod
  conduit list processors filter_processor`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		printAll()
		return nil
	},
	// Silence errors because our logger will catch and print any errors
	SilenceErrors: true,
}

func makeDetailsCommand(pluginType string, data func() []plugins.Metadata) *cobra.Command {
	return &cobra.Command{
		Use:     pluginType + "s",
		Aliases: []string{pluginType},
		Short:   fmt.Sprintf("Usage details for %s plugins.", pluginType),
		Long:    fmt.Sprintf(`Usage details for %s plugins. Pass in a specific plugin as a positional argument for a sample configuration file.`, pluginType),
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				printMetadata(os.Stdout, data(), 0)
			} else {
				printDetails(args[0], data())
			}
		},
	}
}

func init() {
	Command.AddCommand(makeDetailsCommand("importer", pipeline.ImporterMetadata))
	Command.AddCommand(makeDetailsCommand("processor", pipeline.ProcessorMetadata))
	Command.AddCommand(makeDetailsCommand("exporter", pipeline.ExporterMetadata))
}

func printDetails(name string, plugins []plugins.Metadata) {
	for _, data := range plugins {
		if data.Name == name {
			fmt.Println(data.SampleConfig)
			return
		}
	}

	fmt.Printf("Plugin not found: %s\n", name)
}

func printMetadata(w io.Writer, plugins []plugins.Metadata, leftIndent int) {
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Name < plugins[j].Name
	})

	largestPluginNameSize := 0
	for _, data := range plugins {
		if len(data.Name) > largestPluginNameSize {
			largestPluginNameSize = len(data.Name)
		}
	}

	for _, data := range plugins {
		depthStr := strings.Repeat(" ", leftIndent)
		// pad with right with spaces
		fmtString := fmt.Sprintf("%s%%-%ds - %%s\n", depthStr, largestPluginNameSize)

		if data.Deprecated {
			fmtString = "[DEPRECATED] " + fmtString
		}

		fmt.Fprintf(w, fmtString, data.Name, data.Description)
	}
}

func printAll() {
	fmt.Fprint(os.Stdout, "importers:\n")
	printMetadata(os.Stdout, pipeline.ImporterMetadata(), 2)
	fmt.Fprint(os.Stdout, "\nprocessors:\n")
	printMetadata(os.Stdout, pipeline.ProcessorMetadata(), 2)
	fmt.Fprint(os.Stdout, "\nexporters:\n")
	printMetadata(os.Stdout, pipeline.ExporterMetadata(), 2)
}
