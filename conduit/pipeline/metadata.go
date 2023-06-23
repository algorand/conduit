package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters"
	"github.com/algorand/conduit/conduit/plugins/importers"
	"github.com/algorand/conduit/conduit/plugins/processors"
)

// state contains the pipeline state.
type state struct {
	GenesisHash string `json:"genesis-hash"`
	Network     string `json:"network"`
	NextRound   uint64 `json:"next-round"`
	TelemetryID string `json:"telemetry-id,omitempty"`
}

// encodeToFile writes the state object to the dataDir
func (s *state) encodeToFile(dataDir string) error {
	pipelineMetadataFilePath := metadataPath(dataDir)
	tempFilename := fmt.Sprintf("%s.temp", pipelineMetadataFilePath)
	file, err := os.Create(tempFilename)
	if err != nil {
		return fmt.Errorf("encodeMetadataToFile(): failed to create temp metadata file: %w", err)
	}
	defer file.Close()
	err = json.NewEncoder(file).Encode(s)
	if err != nil {
		return fmt.Errorf("encodeMetadataToFile(): failed to write temp metadata: %w", err)
	}

	err = os.Rename(tempFilename, pipelineMetadataFilePath)
	if err != nil {
		return fmt.Errorf("encodeMetadataToFile(): failed to replace metadata file: %w", err)
	}
	return nil
}

// readBlockMetadata attempts to deserialize state from the provided directory
func readBlockMetadata(dataDir string) (state, error) {
	var metadata state
	pipelineMetadataFilePath := metadataPath(dataDir)
	populated, err := isFilePopulated(pipelineMetadataFilePath)
	if err != nil || !populated {
		return metadata, err
	}
	var data []byte
	data, err = os.ReadFile(pipelineMetadataFilePath)
	if err != nil {
		return metadata, fmt.Errorf("error reading metadata: %w", err)
	}
	err = json.Unmarshal(data, &metadata)
	if err != nil {
		return metadata, fmt.Errorf("error reading metadata: %w", err)
	}
	return metadata, err
}

// isFilePopulated returns a bool denoting whether the file at path exists w/ non-zero size
func isFilePopulated(path string) (bool, error) {
	stat, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) || (stat != nil && stat.Size() == 0) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, err
}

// metadataPath returns the canonical path for the serialized pipeline state
func metadataPath(dataDir string) string {
	return path.Join(dataDir, "metadata.json")
}

// AllMetadata gets a slice with metadata from all registered plugins.
func AllMetadata() (results []plugins.Metadata) {
	results = append(results, ImporterMetadata()...)
	results = append(results, ProcessorMetadata()...)
	results = append(results, ExporterMetadata()...)
	return
}

// ImporterMetadata gets a slice with metadata for all importers.Importer plugins.
func ImporterMetadata() (results []plugins.Metadata) {
	for _, constructor := range importers.Importers {
		plugin := constructor.New()
		results = append(results, plugin.Metadata())
	}
	return
}

// ProcessorMetadata gets a slice with metadata for all importers.Processor plugins.
func ProcessorMetadata() (results []plugins.Metadata) {
	for _, constructor := range processors.Processors {
		plugin := constructor.New()
		results = append(results, plugin.Metadata())
	}
	return
}

// ExporterMetadata gets a slice with metadata for all importers.Exporter plugins.
func ExporterMetadata() (results []plugins.Metadata) {
	for _, constructor := range exporters.Exporters {
		plugin := constructor.New()
		results = append(results, plugin.Metadata())
	}
	return
}
