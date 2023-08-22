package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"

	"github.com/algorand/conduit/conduit/pipeline"
)

type mockStatusProvider struct {
	s   pipeline.Status
	err error
}

func (m mockStatusProvider) Status() (pipeline.Status, error) {
	if m.err != nil {
		return pipeline.Status{}, m.err
	}
	return m.s, nil
}

func TestStartServer_BadAddress(t *testing.T) {
	l, h := test.NewNullLogger()
	// capture the fatal log if any.
	l.ExitFunc = func(int) {}
	sp := mockStatusProvider{}

	shutdown, err := StartServer(l, sp, "bad address")
	defer shutdown(context.Background())
	require.NoError(t, err)
	time.Sleep(1 * time.Millisecond)

	require.Len(t, h.Entries, 1)
	require.Equal(t, h.LastEntry().Level, logrus.FatalLevel)
}

func TestStartServer_GracefulShutdown(t *testing.T) {
	l, h := test.NewNullLogger()
	// capture the fatal log if any.
	l.ExitFunc = func(int) {}
	sp := mockStatusProvider{}
	shutdown, err := StartServer(l, sp, "bad address")
	defer shutdown(context.Background())
	require.NoError(t, err)
	require.Len(t, h.Entries, 0)
}

func TestStartServer_HealthCheck(t *testing.T) {
	l, _ := test.NewNullLogger()
	// capture the fatal log if any.
	l.ExitFunc = func(int) {}
	sp := mockStatusProvider{
		s: pipeline.Status{
			Round: 999,
		},
	}

	// Find an open port...
	listener, err := net.Listen("tcp", ":0")
	addr := listener.Addr().String()
	listener.Close()
	require.NoError(t, err)

	// Start server.
	shutdown, err := StartServer(l, sp, addr)
	defer shutdown(context.Background())
	require.NoError(t, err)

	// Make request.
	resp, err := http.Get("http://" + addr + "/health")
	require.NoError(t, err)

	// Make sure we got the right response.
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var respStatus pipeline.Status
	json.NewDecoder(resp.Body).Decode(&respStatus)
	require.NoError(t, err)
	require.Equal(t, sp.s, respStatus)
}

func TestHealthHandlerError(t *testing.T) {
	sp := mockStatusProvider{
		err: fmt.Errorf("some error"),
	}
	handler := makeHealthHandler(sp)
	rec := httptest.NewRecorder()
	handler(rec, nil)

	// validate response
	resp := rec.Result()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Contains(t, rec.Body.String(), "some error")
}
