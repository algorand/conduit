package telemetry

import (
	"time"

	"github.com/opensearch-project/opensearch-go"
)

// TelemetryConfig represents the configuration of Telemetry logging
type TelemetryConfig struct {
	Enable    bool
	SendToLog bool
	URI       string
	Name      string
	GUID      string
	Index     string
	UserName  string
	Password  string
}

// TelemetryEvent represents a single Telemetry event to be emitted to OpenSearch
type TelemetryEvent struct {
	// Contains all the fields set by the user.
	Data map[string]interface{}
	// Time at which the event was created
	Time time.Time
	// Event message
	Message string
	// err may contain a field formatting error
	err string
}

// TelemetryState holds the OpenSearch client and TelemetryConfig
type TelemetryState struct {
	Client          *opensearch.Client
	TelemetryConfig TelemetryConfig
}
