package noop

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

var nc = exporters.ExporterConstructorFunc(func() exporters.Exporter {
	return &noopExporter{}
})
var ne = nc.New()

func TestExporterBuilderByName(t *testing.T) {
	// init() has already registered the noop exporter
	assert.Contains(t, exporters.Exporters, metadata.Name)
	neBuilder, err := exporters.ExporterBuilderByName(metadata.Name)
	assert.NoError(t, err)
	ne := neBuilder.New()
	assert.Implements(t, (*exporters.Exporter)(nil), ne)
}

func TestExporterMetadata(t *testing.T) {
	meta := ne.Metadata()
	assert.Equal(t, metadata.Name, meta.Name)
	assert.Equal(t, metadata.Description, meta.Description)
	assert.Equal(t, metadata.Deprecated, meta.Deprecated)
}

func TestExporterInit(t *testing.T) {
	assert.NoError(t, ne.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(""), nil))
}

func TestExporterClose(t *testing.T) {
	assert.NoError(t, ne.Close())
}

func TestExporterRoundReceive(t *testing.T) {
	eData := data.BlockData{
		BlockHeader: sdk.BlockHeader{
			Round: 5,
		},
	}
	assert.NoError(t, ne.Receive(eData))
}
