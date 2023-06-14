package postgresql

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	_ "github.com/algorand/indexer/idb/dummy"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters"
	"github.com/algorand/conduit/conduit/plugins/exporters/postgresql/util"
)

var pgsqlConstructor = exporters.ExporterConstructorFunc(func() exporters.Exporter {
	return &postgresqlExporter{}
})
var logger *logrus.Logger
var round = sdk.Round(0)

func init() {
	logger, _ = test.NewNullLogger()
}

func TestExporterMetadata(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	meta := pgsqlExp.Metadata()
	assert.Equal(t, metadata.Name, meta.Name)
	assert.Equal(t, metadata.Description, meta.Description)
	assert.Equal(t, metadata.Deprecated, meta.Deprecated)
}

func TestConnectDisconnectSuccess(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.MakePluginConfig("test: true\nconnection-string: ''")
	assert.NoError(t, pgsqlExp.Init(context.Background(), conduit.MakePipelineInitProvider(&round, &sdk.Genesis{}, nil), cfg, logger))
	assert.NoError(t, pgsqlExp.Close())
}

func TestConnectUnmarshalFailure(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.MakePluginConfig("'")
	assert.ErrorContains(t, pgsqlExp.Init(context.Background(), conduit.MakePipelineInitProvider(&round, nil, nil), cfg, logger), "connect failure in unmarshalConfig")
}

func TestConnectDbFailure(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.MakePluginConfig("")
	assert.ErrorContains(t, pgsqlExp.Init(context.Background(), conduit.MakePipelineInitProvider(&round, nil, nil), cfg, logger), "connection string is empty for postgres")
}

func TestReceiveInvalidBlock(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.MakePluginConfig("test: true")
	assert.NoError(t, pgsqlExp.Init(context.Background(), conduit.MakePipelineInitProvider(&round, &sdk.Genesis{}, nil), cfg, logger))
	invalidBlock := data.BlockData{
		BlockHeader: sdk.BlockHeader{
			Round: 1,
		},
		Payset:      sdk.Payset{},
		Certificate: &map[string]interface{}{},
		Delta:       nil,
	}
	err := pgsqlExp.Receive(invalidBlock)
	assert.ErrorIs(t, err, errMissingDelta)
}

func TestReceiveAddBlockSuccess(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.MakePluginConfig("test: true")
	assert.NoError(t, pgsqlExp.Init(context.Background(), conduit.MakePipelineInitProvider(&round, &sdk.Genesis{}, nil), cfg, logger))

	block := data.BlockData{
		BlockHeader: sdk.BlockHeader{},
		Payset:      sdk.Payset{},
		Certificate: &map[string]interface{}{},
		Delta:       &sdk.LedgerStateDelta{},
	}
	assert.NoError(t, pgsqlExp.Receive(block))
}

func TestPostgresqlExporterInit(t *testing.T) {
	pgsqlExp := pgsqlConstructor.New()
	cfg := plugins.MakePluginConfig("test: true")

	// genesis hash mismatch
	initProvider := conduit.MakePipelineInitProvider(&round, &sdk.Genesis{}, nil)
	initProvider.SetGenesis(&sdk.Genesis{
		Network: "test",
	})
	err := pgsqlExp.Init(context.Background(), initProvider, cfg, logger)
	assert.Contains(t, err.Error(), "error importing genesis: genesis hash not matching")

	// incorrect round
	round = 1
	err = pgsqlExp.Init(context.Background(), conduit.MakePipelineInitProvider(&round, &sdk.Genesis{}, nil), cfg, logger)
	assert.Contains(t, err.Error(), "initializing block round 1 but next round to account is 0")
}

func TestUnmarshalConfigsContainingDeleteTask(t *testing.T) {
	// configured delete task
	{
		pgsqlExp := postgresqlExporter{}
		ecfg := ExporterConfig{
			ConnectionString: "",
			MaxConn:          0,
			Test:             true,
			Delete: util.PruneConfigurations{
				Rounds:   3000,
				Interval: 3,
			},
		}
		data, err := yaml.Marshal(ecfg)
		require.NoError(t, err)
		pcfg := plugins.PluginConfig{
			Config: string(data),
		}
		require.NoError(t, pcfg.UnmarshalConfig(&pgsqlExp.cfg))
		assert.Equal(t, 3, int(pgsqlExp.cfg.Delete.Interval))
		assert.Equal(t, uint64(3000), pgsqlExp.cfg.Delete.Rounds)
	}

	// delete task with fields default to 0
	{
		pgsqlExp := postgresqlExporter{}
		cfg := ExporterConfig{
			ConnectionString: "",
			MaxConn:          0,
			Test:             true,
			Delete:           util.PruneConfigurations{},
		}
		data, err := yaml.Marshal(cfg)
		require.NoError(t, err)
		pcfg := plugins.PluginConfig{
			Config: string(data),
		}
		require.NoError(t, pcfg.UnmarshalConfig(&pgsqlExp.cfg))
		assert.Equal(t, 0, int(pgsqlExp.cfg.Delete.Interval))
		assert.Equal(t, uint64(0), pgsqlExp.cfg.Delete.Rounds)
	}

	// delete task with negative interval
	{
		pgsqlExp := postgresqlExporter{}
		cfg := ExporterConfig{
			ConnectionString: "",
			MaxConn:          0,
			Test:             true,
			Delete: util.PruneConfigurations{
				Rounds:   1,
				Interval: -1,
			},
		}
		data, err := yaml.Marshal(cfg)
		require.NoError(t, err)
		pcfg := plugins.PluginConfig{
			Config: string(data),
		}
		require.NoError(t, pcfg.UnmarshalConfig(&pgsqlExp.cfg))
		assert.Equal(t, -1, int(pgsqlExp.cfg.Delete.Interval))
	}

	// delete task with negative round
	{
		pgsqlExp := postgresqlExporter{}
		cfgstr := "test: true\ndelete-task:\n  rounds: -1\n  interval: 2"

		pcfg := plugins.PluginConfig{
			Config: cfgstr,
		}
		assert.ErrorContains(t, pcfg.UnmarshalConfig(&pgsqlExp.cfg), "unmarshal errors")
	}
}
