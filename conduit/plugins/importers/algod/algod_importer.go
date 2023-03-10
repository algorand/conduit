package algodimporter

import (
	"context"
	_ "embed" // used to embed config
	"fmt"
	"net/url"
	"reflect"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/client/v2/common"
	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
	"github.com/algorand/conduit/conduit/plugins/importers"
)

const (
	// PluginName to use when configuring.
	PluginName      = "algod"
	archivalModeStr = "archival"
	followerModeStr = "follower"
)

const (
	archivalMode = iota
	followerMode
)

// Retry
const (
	retries = 5
)

type algodImporter struct {
	aclient *algod.Client
	logger  *logrus.Logger
	cfg     Config
	ctx     context.Context
	cancel  context.CancelFunc
	mode    int
}

//go:embed sample.yaml
var sampleConfig string

var algodImporterMetadata = conduit.Metadata{
	Name:         PluginName,
	Description:  "Importer for fetching blocks from an algod REST API.",
	Deprecated:   false,
	SampleConfig: sampleConfig,
}

func (algodImp *algodImporter) OnComplete(input data.BlockData) error {
	if algodImp.mode != followerMode {
		return nil
	}
	_, err := algodImp.aclient.SetSyncRound(input.Round() + 1).Do(algodImp.ctx)
	return err
}

func (algodImp *algodImporter) Metadata() conduit.Metadata {
	return algodImporterMetadata
}

// package-wide init function
func init() {
	importers.Register(PluginName, importers.ImporterConstructorFunc(func() importers.Importer {
		return &algodImporter{}
	}))
}

func (algodImp *algodImporter) Init(ctx context.Context, cfg plugins.PluginConfig, logger *logrus.Logger) (*sdk.Genesis, error) {
	algodImp.ctx, algodImp.cancel = context.WithCancel(ctx)
	algodImp.logger = logger
	err := cfg.UnmarshalConfig(&algodImp.cfg)
	if err != nil {
		return nil, fmt.Errorf("connect failure in unmarshalConfig: %v", err)
	}

	// To support backwards compatibility with the daemon we default to archival mode
	if algodImp.cfg.Mode == "" {
		algodImp.cfg.Mode = archivalModeStr
	}

	switch algodImp.cfg.Mode {
	case archivalModeStr:
		algodImp.mode = archivalMode
	case followerModeStr:
		algodImp.mode = followerMode
	default:
		return nil, fmt.Errorf("algod importer was set to a mode (%s) that wasn't supported", algodImp.cfg.Mode)
	}

	var client *algod.Client
	u, err := url.Parse(algodImp.cfg.NetAddr)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		algodImp.cfg.NetAddr = "http://" + algodImp.cfg.NetAddr
		algodImp.logger.Infof("Algod Importer added http prefix to NetAddr: %s", algodImp.cfg.NetAddr)
	}
	client, err = algod.MakeClient(algodImp.cfg.NetAddr, algodImp.cfg.Token)
	if err != nil {
		return nil, err
	}
	algodImp.aclient = client

	genesisResponse, err := client.GetGenesis().Do(ctx)
	if err != nil {
		return nil, err
	}

	genesis := sdk.Genesis{}

	// Don't fail on unknown properties here since the go-algorand and SDK genesis types differ slightly
	err = json.LenientDecode([]byte(genesisResponse), &genesis)
	if err != nil {
		return nil, err
	}
	if reflect.DeepEqual(genesis, sdk.Genesis{}) {
		return nil, fmt.Errorf("unable to fetch genesis file from API at %s", algodImp.cfg.NetAddr)
	}

	return &genesis, err
}

func (algodImp *algodImporter) Config() string {
	s, _ := yaml.Marshal(algodImp.cfg)
	return string(s)
}

func (algodImp *algodImporter) Close() error {
	if algodImp.cancel != nil {
		algodImp.cancel()
	}
	return nil
}

func (algodImp *algodImporter) getDelta(rnd uint64) (sdk.LedgerStateDelta, error) {
	var delta sdk.LedgerStateDelta
	params := struct {
		Format string `url:"format,omitempty"`
	}{Format: "msgp"}
	bytes, err := (*common.Client)(algodImp.aclient).GetRaw(algodImp.ctx, fmt.Sprintf("/v2/deltas/%d", rnd), params, nil)
	if err != nil {
		return delta, err
	}

	err = msgpack.Decode(bytes, &delta)
	return delta, err
}

func (algodImp *algodImporter) GetBlock(rnd uint64) (data.BlockData, error) {
	var blockbytes []byte
	var err error
	var status models.NodeStatus
	var blk data.BlockData

	for r := 0; r < retries; r++ {
		status, err = algodImp.aclient.StatusAfterBlock(rnd - 1).Do(algodImp.ctx)
		if err != nil {
			// If context has expired.
			if algodImp.ctx.Err() != nil {
				return blk, fmt.Errorf("GetBlock ctx error: %w", err)
			}
			err = fmt.Errorf("error getting status for round: %w", err)
			algodImp.logger.Errorf("error getting status for round %d (attempt %d)", rnd, r)
			continue
		}
		start := time.Now()
		blockbytes, err = algodImp.aclient.BlockRaw(rnd).Do(algodImp.ctx)
		dt := time.Since(start)
		getAlgodRawBlockTimeSeconds.Observe(dt.Seconds())
		if err != nil {
			algodImp.logger.Errorf("error getting block for round %d (attempt %d)", rnd, r)
			continue
		}
		tmpBlk := new(models.BlockResponse)
		err = msgpack.Decode(blockbytes, tmpBlk)
		if err != nil {
			return blk, err
		}

		blk.BlockHeader = tmpBlk.Block.BlockHeader
		blk.Payset = tmpBlk.Block.Payset
		blk.Certificate = tmpBlk.Cert

		if algodImp.mode == followerMode {
			// We aren't going to do anything with the new delta until we get everything
			// else converted over
			// Round 0 has no delta associated with it
			if rnd != 0 {
				var delta sdk.LedgerStateDelta
				delta, err = algodImp.getDelta(rnd)
				if err != nil {
					if status.LastRound < rnd {
						err = fmt.Errorf("ledger state delta not found: node round (%d) is behind required round (%d), ensure follower node has its sync round set to the required round", status.LastRound, rnd)
					} else {
						err = fmt.Errorf("ledger state delta not found: node round (%d), required round (%d): verify follower node configuration and ensure follower node has its sync round set to the required round, re-deploying the follower node may be necessary", status.LastRound, rnd)
					}
					algodImp.logger.Error(err.Error())
					return data.BlockData{}, err
				}
				blk.Delta = &delta
			}
		}

		return blk, err
	}

	err = fmt.Errorf("failed to get block for round %d after %d attempts, check node configuration: %s", rnd, retries, err)
	algodImp.logger.Errorf(err.Error())
	return blk, err
}

func (algodImp *algodImporter) ProvideMetrics(subsystem string) []prometheus.Collector {
	getAlgodRawBlockTimeSeconds = initGetAlgodRawBlockTimeSeconds(subsystem)
	return []prometheus.Collector{
		getAlgodRawBlockTimeSeconds,
	}
}
