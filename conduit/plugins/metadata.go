package plugins

// Metadata returns fields relevant to identification and description of plugins.
type Metadata struct {
	Name         string
	Description  string
	Deprecated   bool
	SampleConfig string
}

// PluginMetadata is the common interface for providing plugin metadata.
type PluginMetadata interface {
	// Metadata associated with the plugin.
	Metadata() Metadata
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
