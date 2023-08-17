package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/algorand/conduit/conduit/pipeline"
)

// StatusProvider is a subset of the Pipeline interface required by the health handler.
type StatusProvider interface {
	Status() (pipeline.Status, error)
}

func makeHealthHandler(p StatusProvider) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := p.Status()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error": "%s"}`, err)
			return
		}
		w.WriteHeader(http.StatusOK)
		data, _ := json.Marshal(status)
		fmt.Fprint(w, string(data))
	}
}

// StartServer starts an http server that exposes a health check endpoint.
// A callback is returned that can be used to gracefully shutdown the server.
func StartServer(logger *log.Logger, p StatusProvider, address string) (func(ctx context.Context), error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", makeHealthHandler(p))

	srv := &http.Server{
		Addr:    address,
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("failed to start API server: %s", err)
		}
	}()

	shutdownCallback := func(ctx context.Context) {
		if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatalf("failed to shutdown API server: %s", err)
		}
	}
	return shutdownCallback, nil
}
