package conduit

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
)

// RoundRequestor is an optional interface. Plugins should implement it if
// they would like to request an override to the pipeline managed round.
type RoundRequestor interface {
	// RoundRequest is called by the Conduit framework prior to `Init`.
	// This allows plugins with specific round requirements to override
	// the automatic round tracking provided by the Conduit pipeline.
	//
	// There are a few cases where this can be problematic. In these cases
	// an error will be generated and Conduit will fail to launch:
	// 1. Conduit is launched with --next-round-override
	// 2. Multiple plugins request a round override and the requested
	//    rounds are different.
	//
	// If it becomes common for multiple round overrides to be provided,
	// we will add an option to select which override to use. For example,
	// the override rule could be "command-line", "oldest", or the name
	// of a plugin which should be preferred.
	RoundRequest(config plugins.PluginConfig) (uint64, error)
}

// ProvideMetricsFunc is the signature for the PluginMetrics interface.
type ProvideMetricsFunc func() []prometheus.Collector

// PluginMetrics is an optional interface. Plugins should implement it if
// they have custom metrics which should be registered.
type PluginMetrics interface {
	// ProvideMetrics is called by the Conduit framework after the `Init`
	// phase, and before the pipeline starts. The metric collectors are
	// provided to the metrics endpoint which is optionally enabled in the
	// Conduit configuration file.
	ProvideMetrics(subsystem string) []prometheus.Collector
}

// OnCompleteFunc is the signature for the Completed interface.
type OnCompleteFunc func(input data.BlockData) error

// Completed is an optional interface. Plugins should implement it if they
// care to be notified after a round has been fully processed. It can be
// used for things like finalizing state.
type Completed interface {
	// OnComplete is called by the Conduit framework when the pipeline
	// finishes processing a round. It will not be invoked unless all plugins
	// have successfully processed the given round.
	OnComplete(input data.BlockData) error
}
