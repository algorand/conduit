package importers

import (
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// MockImporter and MockImporterConstructor:

type mockImporter struct{}

func (i *mockImporter) Metadata() plugins.Metadata {
	return plugins.Metadata{
		Name:         "Awesome Importer",
		Description:  "",
		Deprecated:   false,
		SampleConfig: "",
	}
}
func (i *mockImporter) Init(ctx context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig, logger *logrus.Logger) (*sdk.Genesis, error) {
	return &sdk.Genesis{}, nil
}
func (i *mockImporter) Config() string                              { return "" }
func (i *mockImporter) Close() error                                { return nil }
func (i *mockImporter) GetBlock(rnd uint64) (data.BlockData, error) { return data.BlockData{}, nil }

type mockImporterConstructor struct{}

func (c *mockImporterConstructor) New() Importer {
	return &mockImporter{}
}

// TestRegister verifies that Register works as expected.
func TestRegister(t *testing.T) {
	mockName := "____mock"
	assert.NotContains(t, Importers, mockName)

	Register(mockName, &mockImporterConstructor{})
	assert.Contains(t, Importers, mockName)

	panicMsg := fmt.Sprintf("importer %s already registered", mockName)
	assert.PanicsWithError(t, panicMsg, func() {
		Register(mockName, &mockImporterConstructor{})
	})
}
