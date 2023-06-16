package telemetry

import (
	"time"

	"github.com/opensearch-project/opensearch-go/v2"
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
	// Unique ID assigned to the pipeline upon initialization
	GUID string `json:"guid"`
	// Version of conduit
	Version string `json:"version"`
}

// Client represents the Telemetry client and config
type Client interface {
	MakeTelemetryStartupEvent() Event
	SendEvent(event Event) error
}

// OpenSearchClient holds the OpenSearch client and TelemetryConfig
type OpenSearchClient struct {
	Client          *opensearch.Client
	TelemetryConfig Config
}
