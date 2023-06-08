package example

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters"
)

var exCons = exporters.ExporterConstructorFunc(func() exporters.Exporter {
	return &exampleExporter{}
})

var exExp = exCons.New()

func TestExporterMetadata(t *testing.T) {
	meta := exExp.Metadata()
	assert.Equal(t, metadata.Name, meta.Name)
	assert.Equal(t, metadata.Description, meta.Description)
	assert.Equal(t, metadata.Deprecated, meta.Deprecated)
}

func TestExporterInit(t *testing.T) {
	assert.Panics(t, func() { exExp.Init(context.Background(), nil, plugins.MakePluginConfig(""), nil) })
}

func TestExporterClose(t *testing.T) {
	assert.Panics(t, func() { exExp.Close() })
}

func TestExporterReceive(t *testing.T) {
	assert.Panics(t, func() { exExp.Receive(data.BlockData{}) })
}
