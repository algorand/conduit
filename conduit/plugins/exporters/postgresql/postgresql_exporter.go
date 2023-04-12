package postgresql

import (
	"context"
	_ "embed" // used to embed config
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
	"github.com/algorand/indexer/idb"
	_ "github.com/algorand/indexer/idb/postgres" // register driver
	"github.com/algorand/indexer/types"
	iutil "github.com/algorand/indexer/util"

	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/exporters"
	"github.com/algorand/conduit/conduit/plugins/exporters/postgresql/util"
)

// PluginName to use when configuring.
const PluginName = "postgresql"

var errMissingDelta = errors.New("ledger state delta is missing from block, ensure algod importer is using 'follower' mode")

type postgresqlExporter struct {
	round  uint64
	cfg    ExporterConfig
	db     idb.IndexerDb
	logger *logrus.Logger
	wg     sync.WaitGroup
	ctx    context.Context
	cf     context.CancelFunc
	dm     util.DataManager
}

//go:embed sample.yaml
var sampleConfig string

var metadata = plugins.Metadata{
	Name:         PluginName,
	Description:  "Exporter for writing data to a postgresql instance.",
	Deprecated:   false,
	SampleConfig: sampleConfig,
}

func (exp *postgresqlExporter) Metadata() plugins.Metadata {
	return metadata
}

// createIndexerDB common code for creating the IndexerDb instance.
func createIndexerDB(logger *logrus.Logger, readonly bool, cfg plugins.PluginConfig) (idb.IndexerDb, chan struct{}, error) {
	var eCfg ExporterConfig
	if err := cfg.UnmarshalConfig(&eCfg); err != nil {
		return nil, nil, fmt.Errorf("connect failure in unmarshalConfig: %v", err)
	}

	// Inject a dummy db for unit testing
	dbName := "postgres"
	if eCfg.Test {
		dbName = "dummy"
	}
	var opts idb.IndexerDbOptions
	opts.MaxConn = eCfg.MaxConn
	opts.ReadOnly = readonly

	// for some reason when ConnectionString is empty, it's automatically
	// connecting to a local instance that's running.
	// this behavior can be reproduced in TestConnectDbFailure.
	if !eCfg.Test && eCfg.ConnectionString == "" {
		return nil, nil, fmt.Errorf("connection string is empty for %s", dbName)
	}
	db, ready, err := idb.IndexerDbByName(dbName, eCfg.ConnectionString, opts, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("connect failure constructing db, %s: %v", dbName, err)
	}

	return db, ready, nil
}

// RoundRequest connects to the database, queries the round, and closes the
// connection. If there is a problem with the configuration, an error will be
// returned in the Init phase and this function will return 0.
func (exp *postgresqlExporter) RoundRequest(cfg plugins.PluginConfig) (uint64, error) {
	nullLogger := logrus.New()
	nullLogger.Out = io.Discard // no logging

	db, _, err := createIndexerDB(nullLogger, true, cfg)
	if err != nil {
		// Assume the error is related to an uninitialized DB.
		// If it is something more serious, the failure will be detected during Init.
		return 0, nil
	}

	rnd, err := db.GetNextRoundToAccount()
	// ignore non initialized error because that would happen during Init.
	// This case probably wont be hit except in very unusual cases because
	// in most cases `createIndexerDB` would fail first.
	if err != nil && err != idb.ErrorNotInitialized {
		return 0, fmt.Errorf("postgres.RoundRequest(): failed to get next round: %w", err)
	}

	return rnd, nil
}

func (exp *postgresqlExporter) Init(ctx context.Context, initProvider data.InitProvider, cfg plugins.PluginConfig, logger *logrus.Logger) error {
	exp.ctx, exp.cf = context.WithCancel(ctx)
	exp.logger = logger

	db, ready, err := createIndexerDB(exp.logger, false, cfg)
	if err != nil {
		return fmt.Errorf("db create error: %v", err)
	}
	<-ready

	exp.db = db
	_, err = iutil.EnsureInitialImport(exp.db, *initProvider.GetGenesis())
	if err != nil {
		return fmt.Errorf("error importing genesis: %v", err)
	}
	dbRound, err := db.GetNextRoundToAccount()
	if err != nil {
		return fmt.Errorf("error getting next db round : %v", err)
	}
	if uint64(initProvider.NextDBRound()) != dbRound {
		return fmt.Errorf("initializing block round %d but next round to account is %d", initProvider.NextDBRound(), dbRound)
	}
	exp.round = uint64(initProvider.NextDBRound())

	// if data pruning is enabled
	if !exp.cfg.Test && exp.cfg.Delete.Rounds > 0 {
		exp.dm = util.MakeDataManager(exp.ctx, &exp.cfg.Delete, exp.db, logger)
		exp.wg.Add(1)
		go exp.dm.DeleteLoop(&exp.wg, &exp.round)
	}
	return nil
}

func (exp *postgresqlExporter) Config() string {
	ret, _ := yaml.Marshal(exp.cfg)
	return string(ret)
}

func (exp *postgresqlExporter) Close() error {
	if exp.db != nil {
		exp.db.Close()
	}

	exp.cf()
	exp.wg.Wait()
	return nil
}

func (exp *postgresqlExporter) Receive(exportData data.BlockData) error {
	if exportData.Delta == nil {
		if exportData.Round() == 0 {
			exportData.Delta = &sdk.LedgerStateDelta{}
		} else {
			return errMissingDelta
		}
	}
	vb := types.ValidatedBlock{
		Block: sdk.Block{BlockHeader: exportData.BlockHeader, Payset: exportData.Payset},
		Delta: *exportData.Delta,
	}
	if err := exp.db.AddBlock(&vb); err != nil {
		return err
	}
	atomic.StoreUint64(&exp.round, exportData.Round()+1)
	return nil
}

func init() {
	exporters.Register(PluginName, exporters.ExporterConstructorFunc(func() exporters.Exporter {
		return &postgresqlExporter{}
	}))
}
