package telemetry

import (
	"time"

	"github.com/opensearch-project/opensearch-go"
)

// Config represents the configuration of Telemetry logging
type Config struct {
	Enable    bool
	SendToLog bool
	URI       string
	Name      string
	GUID      string
	Index     string
	UserName  string
	Password  string
}

// Event represents a single event to be emitted to OpenSearch
type Event struct {
	// Time at which the event was created
	Time time.Time `json:"timestamp"`
	// Event message
	Message string `json:"message"`
	// ID
	GUID string `json:"guid"`
}

// State holds the OpenSearch client and TelemetryConfig
type State struct {
	Client          *opensearch.Client
	TelemetryConfig Config
}
