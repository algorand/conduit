package initialize

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/pipeline"
	"github.com/algorand/conduit/conduit/plugins/exporters/filewriter"
	algodimporter "github.com/algorand/conduit/conduit/plugins/importers/algod"
)

// InitCommand is the init subcommand.
var InitCommand = makeInitCmd()

const defaultDataDirectory = "data"

var errNoWriter = errors.New("configWriter is required")

//go:embed conduit.yml.example
var sampleConfig string

func formatArrayObject(obj string) string {

	var ret string
	lines := strings.Split(obj, "\n")
	for i, line := range lines {
		if i == 0 {
			ret += "  - "
		} else {
			ret += "    "
		}
		ret += line + "\n"
	}

	return ret

}

func runConduitInit(path string, importerFlag string, processorsFlag []string, exporterFlag string) error {
	var location string
	if path == "" {
		path = defaultDataDirectory
		location = "in the current working directory"
	} else {
		location = fmt.Sprintf("at '%s'", path)
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	configFilePath := filepath.Join(path, data.DefaultConfigName)
	f, err := os.Create(configFilePath)
	if err != nil {
		return fmt.Errorf("runConduitInit(): failed to create %s", configFilePath)
	}
	defer f.Close()

	// generate the config file.
	err = writeConfigFile(f, importerFlag, processorsFlag, exporterFlag)
	if err != nil {
		return err
	}

	fmt.Printf("A data directory has been created %s.\n", location)
	fmt.Printf("\nBefore it can be used, the config file needs to be updated with\n")
	fmt.Printf("values for the selected import, export and processor modules. For example,\n")
	fmt.Printf("if the default algod importer was used, set the address/token and the block-dir\n")
	fmt.Printf("path where Conduit should write the block files.\n")
	fmt.Printf("\nOnce the config file is updated, start Conduit with:\n")
	fmt.Printf("  ./conduit -d %s\n", path)

	return nil
}

func writeConfigFile(configWriter io.Writer, importerFlag string, processorsFlag []string, exporterFlag string) error {
	if configWriter == nil {
		return errNoWriter
	}

	var importer string
	if importerFlag == "" {
		importerFlag = algodimporter.PluginName
	}
	for _, metadata := range pipeline.ImporterMetadata() {
		if metadata.Name == importerFlag {
			importer = metadata.SampleConfig
			break
		}
	}
	if importer == "" {
		return fmt.Errorf("runConduitInit(): unknown importer name: %v", importerFlag)
	}

	var exporter string
	if exporterFlag == "" {
		exporterFlag = filewriter.PluginName
	}
	for _, metadata := range pipeline.ExporterMetadata() {
		if metadata.Name == exporterFlag {
			exporter = metadata.SampleConfig
			break
		}
	}
	if exporter == "" {
		return fmt.Errorf("runConduitInit(): unknown exporter name: %v", exporterFlag)
	}

	var processors string
	for _, processorName := range processorsFlag {
		found := false
		for _, metadata := range pipeline.ProcessorMetadata() {
			if metadata.Name == processorName {
				processors = processors + formatArrayObject(metadata.SampleConfig)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("runConduitInit(): unknown processor name: %v", processorName)
		}
	}

	config := fmt.Sprintf(sampleConfig, importer, processors, exporter)

	_, err := configWriter.Write([]byte(config))
	if err != nil {
		return fmt.Errorf("runConduitInit(): failed to write sample config: %w", err)
	}
	return nil
}

// makeInitCmd creates a sample data directory.
func makeInitCmd() *cobra.Command {
	var data string
	var importer string
	var exporter string
	var processors []string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "initializes a Conduit data directory",
		Long: `Initializes a conduit.yml file and writes it to stdout. By default
the config file uses an algod importer in follower mode and a block
file writer exporter. The plugin templates can be changed using the
different options.

Once initialized the conduit.yml file needs to be modified. Refer to the file
comments for details.

If the 'data' option is used, the file will be written to that
directory and additional help is written to stdout.

Once configured, launch conduit with './conduit -d /path/to/data'.`,
		Example: "conduit init  -d /path/to/data -i importer -p processor1,processor2 -e exporter",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if data == "" {
				return writeConfigFile(os.Stdout, importer, processors, exporter)
			}
			return runConduitInit(data, importer, processors, exporter)
		},
		SilenceUsage: true,
	}

	cmd.Flags().StringVarP(&data, "data", "d", "", "Full path to new data directory to initialize. If not set, the conduit configuration YAML is written to stdout.")
	cmd.Flags().StringVarP(&importer, "importer", "i", "", "data importer name.")
	cmd.Flags().StringSliceVarP(&processors, "processors", "p", []string{}, "comma-separated list of processors.")
	cmd.Flags().StringVarP(&exporter, "exporter", "e", "", "data exporter name.")
	return cmd
}
