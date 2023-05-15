package telemetry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTelemetryConfig(t *testing.T) {
	config := MakeTelemetryConfig()
	require.Equal(t, true, config.Enable)
	require.Equal(t, DefaultOpenSearchURI, config.URI)
	require.Equal(t, DefaultIndexName, config.Index)
	require.Equal(t, DefaultTelemetryUserName, config.UserName)
	require.Equal(t, DefaultTelemetryPassword, config.Password)
	require.NotEqual(t, "", config.GUID)
}

func TestMakeTelemetryStartupEvent(t *testing.T) {
	config := Config{
		GUID: "test-guid",
	}
	state, err := MakeTelemetryState(config)
	require.NoError(t, err)
	event := state.MakeTelemetryStartupEvent()
	require.Equal(t, "starting conduit", event.Message)
	require.Equal(t, "test-guid", event.GUID)
}
