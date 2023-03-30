package initialize

import (
	"bytes"
	_ "embed"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/algorand/conduit/conduit/pipeline"
	"github.com/algorand/conduit/conduit/plugins/exporters/filewriter"
	noopExporter "github.com/algorand/conduit/conduit/plugins/exporters/noop"
	algodimporter "github.com/algorand/conduit/conduit/plugins/importers/algod"
	fileimporter "github.com/algorand/conduit/conduit/plugins/importers/filereader"
	"github.com/algorand/conduit/conduit/plugins/processors/filterprocessor"
	noopProcessor "github.com/algorand/conduit/conduit/plugins/processors/noop"
)

//go:embed conduit.test.init.default.yml
var defaultYml string

// TestInitDataDirectory tests the initialization of the data directory
func TestInitDataDirectory(t *testing.T) {
	verifyYaml := func(data []byte, importer string, exporter string, processors []string) {
		var cfg pipeline.Config
		require.NoError(t, yaml.Unmarshal(data, &cfg))
		assert.Equal(t, importer, cfg.Importer.Name)
		assert.Equal(t, exporter, cfg.Exporter.Name)
		require.Equal(t, len(processors), len(cfg.Processors))
		for i := range processors {
			assert.Equal(t, processors[i], cfg.Processors[i].Name)
		}
	}
	verifyFile := func(file string, importer string, exporter string, processors []string) {
		require.FileExists(t, file)
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		verifyYaml(data, importer, exporter, processors)
	}

	// Defaults
	dataDirectory := t.TempDir()
	err := runConduitInit(dataDirectory, "", []string{}, "")
	require.NoError(t, err)
	verifyFile(fmt.Sprintf("%s/conduit.yml", dataDirectory), algodimporter.PluginName, filewriter.PluginName, nil)

	// Explicit defaults
	dataDirectory = t.TempDir()
	err = runConduitInit(dataDirectory, algodimporter.PluginName, []string{noopProcessor.PluginName}, filewriter.PluginName)
	require.NoError(t, err)
	verifyFile(fmt.Sprintf("%s/conduit.yml", dataDirectory), algodimporter.PluginName, filewriter.PluginName, []string{noopProcessor.PluginName})

	// Different
	dataDirectory = t.TempDir()
	err = runConduitInit(dataDirectory, fileimporter.PluginName, []string{noopProcessor.PluginName, filterprocessor.PluginName}, noopExporter.PluginName)
	require.NoError(t, err)
	verifyFile(fmt.Sprintf("%s/conduit.yml", dataDirectory), fileimporter.PluginName, noopExporter.PluginName, []string{noopProcessor.PluginName, filterprocessor.PluginName})

	// config writer
	var buf bytes.Buffer
	err = writeConfigFile(&buf, fileimporter.PluginName, []string{noopProcessor.PluginName, filterprocessor.PluginName}, noopExporter.PluginName)
	require.NoError(t, err)
	verifyYaml(buf.Bytes(), fileimporter.PluginName, noopExporter.PluginName, []string{noopProcessor.PluginName, filterprocessor.PluginName})
}

func TestBadInput(t *testing.T) {
	err := writeConfigFile(nil, "", []string{}, "")
	require.ErrorIs(t, err, errNoWriter)
}
