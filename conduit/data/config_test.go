package data

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPipelineConfigValidity tests the Valid() function for the Config
func TestPipelineConfigValidity(t *testing.T) {
	tests := []struct {
		name        string
		toTest      Config
		errContains string
	}{
		{"valid", Config{
			ConduitArgs: &Args{ConduitDataDir: ""},
			LogLevel:    "info",
			Importer:    NameConfigPair{"test", map[string]interface{}{"a": "a"}},
			Processors:  nil,
			Exporter:    NameConfigPair{"test", map[string]interface{}{"a": "a"}},
		}, ""},

		{"valid 2", Config{
			ConduitArgs: &Args{ConduitDataDir: ""},
			LogLevel:    "info",
			Importer:    NameConfigPair{"test", map[string]interface{}{"a": "a"}},
			Processors:  []NameConfigPair{{"test", map[string]interface{}{"a": "a"}}},
			Exporter:    NameConfigPair{"test", map[string]interface{}{"a": "a"}},
		}, ""},

		{"empty config", Config{ConduitArgs: nil}, "Args.Valid(): conduit args were nil"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.toTest.Valid()

			if test.errContains == "" {
				assert.Nil(t, err)
				return
			}

			assert.Contains(t, err.Error(), test.errContains)
		})
	}
}

// TestMakePipelineConfigError tests that making the pipeline configuration with unknown fields causes an error
func TestMakePipelineConfigErrors(t *testing.T) {
	tests := []struct {
		name             string
		invalidConfigStr string
		errorContains    string
	}{
		{"processors not processor", `---
log-level: info
importer:
  name: "algod"
  config:
    netaddr: "http://127.0.0.1:8080"
    token: "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302"
processor:
  - name: "noop"
    config:
      catchpoint: "7560000#3OUX3TLXZNOK6YJXGETKRRV2MHMILF5CCIVZUOJCT6SLY5H2WWTQ"
exporter:
  name: "noop"
  config:
    connectionstring: ""`, "field processor not found"},
		{"exporter not exporters", `---
log-level: info
importer:
  name: "algod"
  config:
    netaddr: "http://127.0.0.1:8080"
    token: "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302"
processors:
  - name: "noop"
    config:
      catchpoint: "7560000#3OUX3TLXZNOK6YJXGETKRRV2MHMILF5CCIVZUOJCT6SLY5H2WWTQ"
exporters:
  name: "noop"
  config:
    connectionstring: ""`, "field exporters not found"},
		{"config not configs", `---
log-level: info
importer:
  name: "algod"
  config:
    netaddr: "http://127.0.0.1:8080"
    token: "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302"
processors:
  - name: "noop"
    config:
      catchpoint: "7560000#3OUX3TLXZNOK6YJXGETKRRV2MHMILF5CCIVZUOJCT6SLY5H2WWTQ"
exporter:
  name: "noop"
  configs:
    connectionstring: ""`, "field configs not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			dataDir := t.TempDir()

			err := os.WriteFile(filepath.Join(dataDir, DefaultConfigName), []byte(test.invalidConfigStr), 0777)
			assert.Nil(t, err)

			cfg := &Args{ConduitDataDir: dataDir}

			_, err = MakePipelineConfig(cfg)
			assert.ErrorContains(t, err, test.errorContains)
		})
	}
}

// TestMakePipelineConfig tests making the pipeline configuration
func TestMakePipelineConfig(t *testing.T) {

	_, err := MakePipelineConfig(nil)
	assert.Equal(t, fmt.Errorf("MakePipelineConfig(): empty conduit config"), err)

	dataDir := t.TempDir()

	validConfigFile := `---
log-level: info
importer:
  name: "algod"
  config:
    netaddr: "http://127.0.0.1:8080"
    token: "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302"
processors:
  - name: "noop"
    config:
      catchpoint: "7560000#3OUX3TLXZNOK6YJXGETKRRV2MHMILF5CCIVZUOJCT6SLY5H2WWTQ"
exporter:
  name: "noop"
  config:
    connectionstring: ""`

	err = os.WriteFile(filepath.Join(dataDir, DefaultConfigName), []byte(validConfigFile), 0777)
	assert.Nil(t, err)

	cfg := &Args{ConduitDataDir: dataDir}

	pCfg, err := MakePipelineConfig(cfg)
	assert.Nil(t, err)
	assert.Equal(t, pCfg.LogLevel, "info")
	assert.Equal(t, pCfg.Valid(), nil)
	assert.Equal(t, pCfg.Importer.Name, "algod")
	assert.Equal(t, pCfg.Importer.Config["token"], "e36c01fc77e490f23e61899c0c22c6390d0fff1443af2c95d056dc5ce4e61302")
	assert.Equal(t, pCfg.Processors[0].Name, "noop")
	assert.Equal(t, pCfg.Processors[0].Config["catchpoint"], "7560000#3OUX3TLXZNOK6YJXGETKRRV2MHMILF5CCIVZUOJCT6SLY5H2WWTQ")
	assert.Equal(t, pCfg.Exporter.Name, "noop")
	assert.Equal(t, pCfg.Exporter.Config["connectionstring"], "")

	// invalidDataDir has no auto load file
	invalidDataDir := t.TempDir()
	assert.Nil(t, err)

	cfgBad := &Args{ConduitDataDir: invalidDataDir}
	_, err = MakePipelineConfig(cfgBad)
	assert.Equal(t, err,
		fmt.Errorf("MakePipelineConfig(): could not find %s in data directory (%s)", DefaultConfigName, cfgBad.ConduitDataDir))

}
