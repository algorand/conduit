package cli

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/algorand/conduit/conduit/data"
	_ "github.com/algorand/conduit/conduit/plugins/exporters/noop"
	_ "github.com/algorand/conduit/conduit/plugins/importers/noop"
)

// Fills in a temp data dir and creates files
// TODO: Refactor the code so that testing can be done without creating files and directories.
func setupDataDir(t *testing.T, cfg data.Config) *data.Args {
	conduitArgs := &data.Args{ConduitDataDir: t.TempDir()}
	ydata, err := yaml.Marshal(&cfg)
	require.NoError(t, err)
	configFile := path.Join(conduitArgs.ConduitDataDir, data.DefaultConfigName)
	os.WriteFile(configFile, ydata, 0755)
	require.FileExists(t, configFile)
	return conduitArgs
}

func TestBanner(t *testing.T) {
	test := func(t *testing.T, hideBanner bool) {
		// Capture stdout.
		stdout := os.Stdout
		defer func() {
			os.Stdout = stdout
		}()
		stdoutFilePath := path.Join(t.TempDir(), "stdout.txt")
		f, err := os.Create(stdoutFilePath)
		require.NoError(t, err)
		defer f.Close()
		os.Stdout = f

		cfg := data.Config{
			ConduitArgs: &data.Args{ConduitDataDir: t.TempDir()},
			HideBanner:  hideBanner,
			Importer:    data.NameConfigPair{Name: "test", Config: map[string]interface{}{"a": "a"}},
			Processors:  nil,
			Exporter:    data.NameConfigPair{Name: "test", Config: map[string]interface{}{"a": "a"}},
		}
		args := setupDataDir(t, cfg)

		err = runConduitCmdWithConfig(args)
		data, err := os.ReadFile(stdoutFilePath)
		require.NoError(t, err)

		if hideBanner {
			assert.NotContains(t, string(data), Banner)
		} else {
			assert.Contains(t, string(data), Banner)
		}
	}

	t.Run("Banner_hidden", func(t *testing.T) {
		test(t, true)
	})

	t.Run("Banner_shown", func(t *testing.T) {
		test(t, false)
	})
}

func TestEmptyDataDir(t *testing.T) {
	args := data.Args{}
	err := runConduitCmdWithConfig(&args)
	require.ErrorContains(t, err, conduitEnvVar)
}

func TestInvalidLogLevel(t *testing.T) {
	cfg := data.Config{
		LogLevel: "invalid",
	}
	args := setupDataDir(t, cfg)
	err := runConduitCmdWithConfig(args)
	require.ErrorContains(t, err, "not a valid log level")
}

func TestLogFile(t *testing.T) {
	// returns stdout
	test := func(t *testing.T, logfile string) ([]byte, error) {
		// Capture stdout.
		stdout := os.Stdout
		defer func() {
			os.Stdout = stdout
		}()
		stdoutFilePath := path.Join(t.TempDir(), "stdout.txt")
		f, err := os.Create(stdoutFilePath)
		require.NoError(t, err)
		defer f.Close()
		os.Stdout = f

		cfg := data.Config{
			LogFile:    logfile,
			Importer:   data.NameConfigPair{Name: "test", Config: map[string]interface{}{"a": "a"}},
			Processors: nil,
			Exporter:   data.NameConfigPair{Name: "test", Config: map[string]interface{}{"a": "a"}},
		}
		args := setupDataDir(t, cfg)

		err = runConduitCmdWithConfig(args)
		require.ErrorContains(t, err, "pipeline creation error")
		return os.ReadFile(stdoutFilePath)
	}

	// logging to stdout
	t.Run("conduit-logging-stdout", func(t *testing.T) {
		data, err := test(t, "")
		require.NoError(t, err)
		dataStr := string(data)
		require.Contains(t, dataStr, "{")
		require.Contains(t, dataStr, "\nWriting logs to console.")
	})

	// logging to file
	t.Run("conduit-logging-file", func(t *testing.T) {
		logfile := path.Join(t.TempDir(), "logfile.txt")
		data, err := test(t, logfile)
		require.NoError(t, err)
		dataStr := string(data)
		require.NotContains(t, dataStr, "{")
		logdata, err := os.ReadFile(logfile)
		require.NoError(t, err)
		logdataStr := string(logdata)
		require.Contains(t, logdataStr, "{")
		// pipeline error is not suppressed from log file.
		require.Contains(t, logdataStr, "pipeline creation error")
		// written to stdout and logfile
		require.Contains(t, dataStr, "\nWriting logs to file:")
	})
}

func TestHealthEndpoint(t *testing.T) {
	healthPort := 7777
	healthNet := fmt.Sprintf("http://localhost:%d/health", healthPort)

	test := func(t *testing.T, address string) {
		cfg := data.Config{
			ConduitArgs: &data.Args{ConduitDataDir: t.TempDir()},
			API:         data.API{Address: address},
			Importer:    data.NameConfigPair{Name: "noop", Config: map[string]interface{}{}},
			Processors:  nil,
			Exporter:    data.NameConfigPair{Name: "noop", Config: map[string]interface{}{}},
		}
		args := setupDataDir(t, cfg)

		go func() {
			runConduitCmdWithConfig(args)
		}()
		time.Sleep(1 * time.Second)

		resp, err := http.Get(healthNet)
		if address != "" {
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
		} else {
			require.ErrorContains(t, err, "connection refused")
			require.Nil(t, resp)
		}
	}

	t.Run("API_OFF", func(t *testing.T) {
		test(t, "")
	})

	t.Run("API_ON", func(t *testing.T) {
		test(t, fmt.Sprintf(":%d", healthPort))
	})
}
