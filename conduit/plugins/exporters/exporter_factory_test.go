package exporters

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"

	"github.com/algorand/conduit/conduit/plugins"
)

var logger *logrus.Logger

func init() {
	logger, _ = test.NewNullLogger()
}

type mockExporter struct {
	Exporter
}

func (m *mockExporter) Metadata() plugins.Metadata {
	return plugins.Metadata{}
}

type mockExporterConstructor struct {
	me *mockExporter
}

func (c *mockExporterConstructor) New() Exporter {
	return c.me
}

func TestExporterByNameSuccess(t *testing.T) {
	me := mockExporter{}
	Register("foobar", &mockExporterConstructor{&me})

	expC, err := ExporterBuilderByName("foobar")
	assert.NoError(t, err)
	exp := expC.New()
	assert.Implements(t, (*Exporter)(nil), exp)
}

func TestExporterByNameNotFound(t *testing.T) {
	_, err := ExporterBuilderByName("barfoo")
	expectedErr := "no Exporter Constructor for barfoo"
	assert.EqualError(t, err, expectedErr)
}

// TestRegister verifies that Register works as expected.
func TestRegister(t *testing.T) {
	mockName := "____mock"
	assert.NotContains(t, Exporters, mockName)

	Register(mockName, &mockExporterConstructor{})
	assert.Contains(t, Exporters, mockName)

	panicMsg := fmt.Sprintf("exporter %s already registered", mockName)
	assert.PanicsWithError(t, panicMsg, func() {
		Register(mockName, &mockExporterConstructor{})
	})
}
