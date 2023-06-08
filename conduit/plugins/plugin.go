package plugins

// Plugin is the common interface for all Conduit plugins.
type Plugin interface {
	// PluginMetadata - implement this interface.
	PluginMetadata

	// Close will be called during termination of the Conduit process.
	// There is no guarantee that plugin lifecycle hooks will be invoked in any specific order in relation to one another.
	// Returns an error if it fails which will be surfaced in the logs, but the process is already terminating.
	Close() error
}
