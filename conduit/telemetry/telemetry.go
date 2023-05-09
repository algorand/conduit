package telemetry

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
)

// MakeOpenSearchClient creates a new OpenSearch client.
func MakeOpenSearchClient(cfg TelemetryConfig) (*opensearch.Client, error) {
	client, err := opensearch.NewClient(opensearch.Config{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Addresses: []string{cfg.URI},
		Username:  cfg.UserName, // For testing only. Don't store credentials in code.
		Password:  cfg.Password,
	})
	if err != nil {
		// TODO: Add more information about configs
		return nil, fmt.Errorf("unable to create new OpenSearch client with URI %s: %w", cfg.URI, err)
	}
	return client, nil
}

// MakeTelemetryState initializes a new TelemetryState.
func MakeTelemetryState(cfg TelemetryConfig) (*TelemetryState, error) {
	client, err := MakeOpenSearchClient(cfg)
	if err != nil {
		return nil, err
	}

	telemetryState := &TelemetryState{
		Client:          client,
		TelemetryConfig: cfg,
	}
	return telemetryState, nil
}

// MakeTelemetryConfig initializes a new TelemetryConfig.
func MakeTelemetryConfig() TelemetryConfig {
	return TelemetryConfig{
		Enable:   true,
		URI:      "https://localhost:9200", // TODO: Fix to actual URI
		GUID:     uuid.New().String(),      // Uses Google UUID instead of go-algorand utils
		Index:    "conduit-telemetry",
		UserName: "admin", // TODO: Use algorand credentials
		Password: "admin",
	}
}

func (t *TelemetryState) MakeTelemetryStartupEvent() map[string]interface{} {
	return map[string]interface{}{
		"message": "starting conduit",
		"guid":    t.TelemetryConfig.GUID,
		"time":    time.Now(),
	}
}

func (t *TelemetryState) SendEvent(event map[string]interface{}) error {
	req := opensearchapi.IndexRequest{
		Index: t.TelemetryConfig.Index,
		Body:  opensearchutil.NewJSONReader(event),
	}
	_, err := req.Do(context.Background(), t.Client)
	if err != nil {
		return fmt.Errorf("failed to insert event ", err)
	}
	return nil
}
