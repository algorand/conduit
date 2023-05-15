package processors

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

type mockProcessor struct {
	Processor
}

func (m *mockProcessor) Metadata() plugins.Metadata {
	return plugins.Metadata{
		Name:         "foobar",
		Description:  "",
		Deprecated:   false,
		SampleConfig: "",
	}
}

type mockProcessorConstructor struct {
	me *mockProcessor
}

func (c *mockProcessorConstructor) New() Processor {
	return c.me
}

func TestProcessorBuilderByNameSuccess(t *testing.T) {
	me := mockProcessor{}
	Register("foobar", &mockProcessorConstructor{&me})

	expBuilder, err := ProcessorBuilderByName("foobar")
	assert.NoError(t, err)
	exp := expBuilder.New()
	assert.Implements(t, (*Processor)(nil), exp)
}

func TestProcessorBuilderByNameNotFound(t *testing.T) {
	_, err := ProcessorBuilderByName("barfoo")
	expectedErr := "no Processor Constructor for barfoo"
	assert.EqualError(t, err, expectedErr)
}

// TestRegister verifies that Register works as expected.
func TestRegister(t *testing.T) {
	mockName := "____mock"
	assert.NotContains(t, Processors, mockName)

	Register(mockName, &mockProcessorConstructor{})
	assert.Contains(t, Processors, mockName)

	panicMsg := fmt.Sprintf("processor %s already registered", mockName)
	assert.PanicsWithError(t, panicMsg, func() {
		Register(mockName, &mockProcessorConstructor{})
	})
}
