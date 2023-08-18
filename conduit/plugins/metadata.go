package plugins

import (
	"fmt"
	"os"
	"path"

	yaml "gopkg.in/yaml.v3"

	"github.com/algorand/conduit/conduit/data"
)

// Metadata returns fields relevant to identification and description of plugins.
type Metadata struct {
	Name         string
	Description  string
	Deprecated   bool
	SampleConfig string
}

// PluginType is defined for each plugin category
type PluginType string

const (
	// Exporter PluginType
	Exporter PluginType = "exporter"

	// Processor PluginType
	Processor PluginType = "processor"

	// Importer PluginType
	Importer PluginType = "importer"
)

// GetConfig creates an appropriate plugin config for the type.
func (pt PluginType) GetConfig(cfg data.NameConfigPair, dataDir string) (PluginConfig, error) {
	configs, err := yaml.Marshal(cfg.Config)
	if err != nil {
		return PluginConfig{}, fmt.Errorf("GetConfig(): could not serialize config: %w", err)
	}

	var config PluginConfig
	config.Config = string(configs)
	if dataDir != "" {
		config.DataDir = path.Join(dataDir, fmt.Sprintf("%s_%s", pt, cfg.Name))
		err = os.MkdirAll(config.DataDir, os.ModePerm)
		if err != nil {
			return PluginConfig{}, fmt.Errorf("GetConfig: unable to create plugin data directory: %w", err)
		}
	}

	return config, nil
}
