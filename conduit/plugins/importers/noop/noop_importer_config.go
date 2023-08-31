package noop

// ImporterConfig specific to the noop importer
type ImporterConfig struct {
	// Optionally specify the round to start on
	Round uint64 `yaml:"round"`
}
