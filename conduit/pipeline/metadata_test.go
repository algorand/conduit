package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/algorand/conduit/conduit/data"
	_ "github.com/algorand/conduit/conduit/plugins/exporters/all"
	_ "github.com/algorand/conduit/conduit/plugins/exporters/example"
	_ "github.com/algorand/conduit/conduit/plugins/importers/all"
	_ "github.com/algorand/conduit/conduit/plugins/processors/all"
)

// TestSamples ensures that all plugins contain a sample file with valid yaml.
func TestSamples(t *testing.T) {
	metadata := AllMetadata()
	for _, mdata := range metadata {
		mdata := mdata
		t.Run(mdata.Name, func(t *testing.T) {
			t.Parallel()
			var config data.NameConfigPair
			assert.NoError(t, yaml.Unmarshal([]byte(mdata.SampleConfig), &config))
			assert.Equal(t, mdata.Name, config.Name)
		})
	}
}

// TestBlockMetaDataFile tests that metadata.json file is created as expected
func TestBlockMetaDataFile(t *testing.T) {
	datadir := t.TempDir()
	pipelineMetadata := state{
		NextRound: 3,
	}

	// Test the file is not created yet
	populated, err := isFilePopulated(metadataPath(datadir))
	assert.NoError(t, err)
	assert.False(t, populated)

	// Write the file
	err = pipelineMetadata.encodeToFile(datadir)
	assert.NoError(t, err)

	// Test that file is created
	populated, err = isFilePopulated(metadataPath(datadir))
	assert.NoError(t, err)
	assert.True(t, populated)

	// Test that file loads correctly
	metaData, err := readBlockMetadata(datadir)
	assert.NoError(t, err)
	assert.Equal(t, pipelineMetadata.GenesisHash, metaData.GenesisHash)
	assert.Equal(t, pipelineMetadata.NextRound, metaData.NextRound)
	assert.Equal(t, pipelineMetadata.Network, metaData.Network)
	assert.Equal(t, pipelineMetadata.TelemetryID, metaData.TelemetryID)

	// Test that file encodes correctly
	pipelineMetadata.GenesisHash = "HASH"
	pipelineMetadata.NextRound = 7
	pipelineMetadata.TelemetryID = "SOME_ID"
	err = pipelineMetadata.encodeToFile(datadir)
	assert.NoError(t, err)
	metaData, err = readBlockMetadata(datadir)
	assert.NoError(t, err)
	assert.Equal(t, "HASH", metaData.GenesisHash)
	assert.Equal(t, uint64(7), metaData.NextRound)
	assert.Equal(t, pipelineMetadata.Network, metaData.Network)
	assert.Equal(t, pipelineMetadata.TelemetryID, metaData.TelemetryID)
}
