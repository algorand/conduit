package plugins

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
	Exporter = "exporter"

	// Processor PluginType
	Processor = "processor"

	// Importer PluginType
	Importer = "importer"
)
