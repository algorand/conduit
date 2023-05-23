package data

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// DefaultConfigBaseName is the default conduit configuration filename without the extension.
var DefaultConfigBaseName = "conduit"

// DefaultConfigName is the default conduit configuration filename.
var DefaultConfigName = fmt.Sprintf("%s.yml", DefaultConfigBaseName)

// DefaultLogLevel is the default conduit log level if none is provided.
var DefaultLogLevel = log.InfoLevel

// DefaultMetricsPrefix is the default prometheus subsystem if no Prefix option is provided.
var DefaultMetricsPrefix = "conduit"

// Args configuration for conduit running.
type Args struct {
	ConduitDataDir    string `yaml:"data-dir"`
	NextRoundOverride uint64 `yaml:"next-round-override"`
}

// NameConfigPair is a generic structure used across plugin configuration ser/de
type NameConfigPair struct {
	Name   string                 `yaml:"name"`
	Config map[string]interface{} `yaml:"config"`
}

// Metrics configs for turning on Prometheus endpoint /metrics
type Metrics struct {
	Mode   string `yaml:"mode"`
	Addr   string `yaml:"addr"`
	Prefix string `yaml:"prefix"`
}

// Telemetry configs for sending Telemetry to OpenSearch
type Telemetry struct {
	Enabled  bool   `yaml:"enabled"`
	URI      string `yaml:"uri"`
	Index    string `yaml:"index"`
	UserName string `yaml:"username"`
	Password string `yaml:"password"`
}

// Config stores configuration specific to the conduit pipeline
type Config struct {
	// ConduitArgs are the program inputs. Should not be serialized for config.
	ConduitArgs *Args `yaml:"-"`

	CPUProfile  string `yaml:"cpu-profile"`
	PIDFilePath string `yaml:"pid-filepath"`
	HideBanner  bool   `yaml:"hide-banner"`

	LogFile  string `yaml:"log-file"`
	LogLevel string `yaml:"log-level"`
	// Store a local copy to access parent variables
	Importer   NameConfigPair   `yaml:"importer"`
	Processors []NameConfigPair `yaml:"processors"`
	Exporter   NameConfigPair   `yaml:"exporter"`
	Metrics    Metrics          `yaml:"metrics"`
	// RetryCount is the number of retries to perform for an error in the pipeline
	RetryCount uint64 `yaml:"retry-count"`
	// RetryDelay is a duration amount interpreted from a string
	RetryDelay time.Duration `yaml:"retry-delay"`

	Telemetry Telemetry `yaml:"telemetry"`
}

// Valid validates pipeline config
func (cfg *Config) Valid() error {
	if cfg.ConduitArgs == nil {
		return fmt.Errorf("Args.Valid(): conduit args were nil")
	}

	// If it is a negative time, it is an error
	if cfg.RetryDelay < 0 {
		return fmt.Errorf("Args.Valid(): invalid retry delay - time duration was negative (%s)", cfg.RetryDelay.String())
	}

	return nil
}

// MakePipelineConfig creates a pipeline configuration
func MakePipelineConfig(args *Args) (*Config, error) {
	if args == nil {
		return nil, fmt.Errorf("MakePipelineConfig(): empty conduit config")
	}

	if !isDir(args.ConduitDataDir) {
		return nil, fmt.Errorf("MakePipelineConfig(): invalid data dir '%s'", args.ConduitDataDir)
	}

	// Search for pipeline configuration in data directory
	autoloadParamConfigPath, err := getConfigFromDataDir(args.ConduitDataDir, DefaultConfigBaseName, []string{"yml", "yaml"})
	if err != nil || autoloadParamConfigPath == "" {
		return nil, fmt.Errorf("MakePipelineConfig(): could not find %s in data directory (%s)", DefaultConfigName, args.ConduitDataDir)
	}

	file, err := os.Open(autoloadParamConfigPath)
	if err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): reading config error: %w", err)
	}

	pCfgDecoder := yaml.NewDecoder(file)
	// Make sure we are strict about only unmarshalling known fields
	pCfgDecoder.KnownFields(true)

	var pCfg Config
	// Set default value for retry variables
	pCfg.RetryDelay = 1 * time.Second
	pCfg.RetryCount = 10
	err = pCfgDecoder.Decode(&pCfg)
	if err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): config file (%s) was mal-formed yaml: %w", autoloadParamConfigPath, err)
	}

	// For convenience, include the command line arguments.
	pCfg.ConduitArgs = args

	// Default log level.
	if pCfg.LogLevel == "" {
		pCfg.LogLevel = DefaultLogLevel.String()
	}

	if err := pCfg.Valid(); err != nil {
		return nil, fmt.Errorf("MakePipelineConfig(): config file (%s) had mal-formed schema: %w", autoloadParamConfigPath, err)
	}

	return &pCfg, nil
}

// isDir returns true if the specified directory is valid. Copied from Indexer util.IsDir
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// fileExists checks to see if the specified file (or directory) exists. Copied from Indexer util.FileExists
func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	fileExists := err == nil
	return fileExists
}

// getConfigFromDataDir Given the data directory, configuration filename and a list of types, see if
// a configuration file that matches was located there.  If no configuration file was there then an
// empty string is returned.  If more than one filetype was matched, an error is returned.
// Copied from Indexer util.GetConfigFromDataDir
func getConfigFromDataDir(dataDirectory string, configFilename string, configFileTypes []string) (string, error) {
	count := 0
	fullPath := ""
	var err error

	for _, configFileType := range configFileTypes {
		autoloadParamConfigPath := filepath.Join(dataDirectory, configFilename+"."+configFileType)
		if fileExists(autoloadParamConfigPath) {
			count++
			fullPath = autoloadParamConfigPath
		}
	}

	if count > 1 {
		return "", fmt.Errorf("config filename (%s) in data directory (%s) matched more than one filetype: %v",
			configFilename, dataDirectory, configFileTypes)
	}

	// if count == 0 then the fullpath will be set to "" and error will be nil
	// if count == 1 then it fullpath will be correct
	return fullPath, err
}
