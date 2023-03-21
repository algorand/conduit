package algodimporter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/plugins"
)

var (
	logger *logrus.Logger
	ctx    context.Context
	cancel context.CancelFunc
	pRound sdk.Round
)

func init() {
	logger = logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)
	ctx, cancel = context.WithCancel(context.Background())
	pRound = sdk.Round(1)
}

// New initializes an algod importer
func New() *algodImporter {
	return &algodImporter{}
}

func TestImporterMetadata(t *testing.T) {
	testImporter := New()
	metadata := testImporter.Metadata()
	assert.Equal(t, metadata.Name, algodImporterMetadata.Name)
	assert.Equal(t, metadata.Description, algodImporterMetadata.Description)
	assert.Equal(t, metadata.Deprecated, algodImporterMetadata.Deprecated)
}

func TestCloseSuccess(t *testing.T) {
	ts := NewAlgodServer(GenesisResponder, MakeSyncRoundResponder(200), BlockAfterResponder)
	testImporter := New()
	cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, archivalModeStr, ts.URL)
	_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
	assert.NoError(t, err)
	err = testImporter.Close()
	assert.NoError(t, err)
}

func TestInitSuccess(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"archival"},
		{"follower"},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ts := NewAlgodServer(GenesisResponder, MakeSyncRoundResponder(200), BlockAfterResponder)
			testImporter := New()
			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ts.URL)
			_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
			assert.NoError(t, err)
			assert.NotEqual(t, testImporter, nil)
			testImporter.Close()
		})
	}
}

func TestInitCatchup(t *testing.T) {
	tests := []struct {
		name        string
		catchpoint  string
		algodServer *httptest.Server
		err         string
		logs        []string
	}{
		{"sync round failure", "",
			NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(400)),
			"received unexpected error setting sync round: HTTP 400",
			[]string{}},
		{"catchpoint parse failure", "notvalid",
			NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(200)),
			"unable to parse catchpoint, invalid format",
			[]string{}},
		{"invalid catchpoint round uint parsing error", "abcd#abcd",
			NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(200)),
			"invalid syntax",
			[]string{}},
		{"node status failure", "1234#abcd",
			NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(200),
				MakeStatusResponder("/v2/status", 400, "")),
			"received unexpected error failed to get node status: HTTP 400",
			[]string{}},
		{"catchpoint round before node round skips fast catchup", "1234#abcd",
			NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(200),
				MakeNodeStatusResponder(models.NodeStatus{LastRound: 1235})),
			"",
			[]string{"Skipping catchpoint catchup for 1234#abcd, since it's before node round 1235"}},
		{"start catchpoint catchup failure", "1236#abcd",
			NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(200),
				MakeNodeStatusResponder(models.NodeStatus{LastRound: 1235}),
				MakeStatusResponder("/v2/catchup/", 400, "")),
			"POST /v2/catchup/1236#abcd received unexpected error: HTTP 400",
			[]string{}},
		{"monitor catchup node status failure", "1236#abcd",
			NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(200),
				MakeJsonResponderSeries("/v2/status", []int{200, 400}, []interface{}{models.NodeStatus{LastRound: 1235}}),
				MakeStatusResponder("/v2/catchup/", 200, "")),
			"received unexpected error getting node status: HTTP 400",
			[]string{}},
		{"monitor catchup success", "1236#abcd",
			NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(200),
				MakeJsonResponderSeries("/v2/status", []int{200}, []interface{}{
					models.NodeStatus{LastRound: 1235},
					models.NodeStatus{Catchpoint: "1236#abcd", CatchpointProcessedAccounts: 1, CatchpointTotalAccounts: 1},
					models.NodeStatus{Catchpoint: "1236#abcd", CatchpointVerifiedAccounts: 1, CatchpointTotalAccounts: 1},
					models.NodeStatus{Catchpoint: "1236#abcd", CatchpointAcquiredBlocks: 1, CatchpointTotalBlocks: 1},
					models.NodeStatus{Catchpoint: "1236#abcd"},
					models.NodeStatus{LastRound: 1236},
				}),
				MakeStatusResponder("/v2/catchup/", 200, "")),
			"",
			[]string{
				"catchup phase 1 of 4 (Processed Accounts): 1 / 1",
				"catchup phase 2 of 4 (Verified Accounts): 1 / 1",
				"catchup phase 3 of 4 (Acquired Blocks): 1 / 1",
				"catchup phase 4 of 4 (Verified Blocks)",
			}},
		{"wait for node to catchup error", "1236#abcd",
			NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(200),
				MakeJsonResponderSeries("/v2/status", []int{200, 200, 400}, []interface{}{models.NodeStatus{LastRound: 1235}}),
				MakeStatusResponder("/v2/catchup/", 200, "")),
			"received unexpected error (StatusAfterBlock) waiting for node to catchup: HTTP 400",
			[]string{}},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			testLogger, hook := test.NewNullLogger()
			testImporter := New()
			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
catchup-config:
  catchpoint: %s
`, "follower", ttest.algodServer.URL, ttest.catchpoint)
			_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), testLogger)
			if ttest.err != "" {
				assert.ErrorContains(t, err, ttest.err)
			} else {
				assert.NoError(t, err)
			}
			_ = testImporter.Close()
			// Make sure each of the expected log messages are present
			for _, log := range ttest.logs {
				found := false
				for _, entry := range hook.AllEntries() {
					found = found || strings.Contains(entry.Message, log)
				}
				assert.True(t, found, "Expected log was not found: '%s'", log)
			}
		})
	}
}

func TestInitParseUrlFailure(t *testing.T) {
	tests := []struct {
		url string
	}{
		{".0.0.0.0.0.0.0:1234"},
	}
	for _, ttest := range tests {
		t.Run(ttest.url, func(t *testing.T) {
			testImporter := New()
			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, "follower", ttest.url)
			_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
			assert.ErrorContains(t, err, "parse")
		})
	}
}

func TestInitModeFailure(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"foobar"},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ts := NewAlgodServer(GenesisResponder)
			testImporter := New()
			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ts.URL)
			_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
			assert.EqualError(t, err, fmt.Sprintf("algod importer was set to a mode (%s) that wasn't supported", ttest.name))
		})
	}
}

func TestInitGenesisFailure(t *testing.T) {
	ts := NewAlgodServer(MakeGenesisResponder(sdk.Genesis{}))
	testImporter := New()
	cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, archivalModeStr, ts.URL)
	_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "unable to fetch genesis file")
	testImporter.Close()
}

func TestInitUnmarshalFailure(t *testing.T) {
	testImporter := New()
	_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig("`"), logger)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "connect failure in unmarshalConfig")
	testImporter.Close()
}

func TestConfigDefault(t *testing.T) {
	testImporter := New()
	expected, err := yaml.Marshal(&Config{})
	if err != nil {
		t.Fatalf("unable to Marshal default algodimporter.Config: %v", err)
	}
	assert.Equal(t, string(expected), testImporter.Config())
}

func TestWaitForBlockBlockFailure(t *testing.T) {
	ts := NewAlgodServer(GenesisResponder, MakeSyncRoundResponder(200), BlockAfterResponder)
	testImporter := New()
	cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, archivalModeStr, ts.URL)
	_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
	assert.NoError(t, err)
	assert.NotEqual(t, testImporter, nil)

	blk, err := testImporter.GetBlock(uint64(10))
	assert.Error(t, err)
	assert.True(t, blk.Empty())

}

func TestGetBlockSuccess(t *testing.T) {
	tests := []struct {
		name        string
		algodServer *httptest.Server
	}{
		{"", NewAlgodServer(GenesisResponder,
			BlockResponder,
			BlockAfterResponder,
			MakeSyncRoundResponder(200))},
		{"archival", NewAlgodServer(GenesisResponder,
			BlockResponder,
			BlockAfterResponder,
			MakeSyncRoundResponder(200))},
		{"follower", NewAlgodServer(GenesisResponder,
			BlockResponder,
			BlockAfterResponder, LedgerStateDeltaResponder, MakeSyncRoundResponder(200))},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ctx, cancel = context.WithCancel(context.Background())
			testImporter := New()

			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ttest.algodServer.URL)
			_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
			assert.NoError(t, err)
			assert.NotEqual(t, testImporter, nil)

			downloadedBlk, err := testImporter.GetBlock(uint64(0))
			assert.NoError(t, err)
			assert.Equal(t, downloadedBlk.Round(), uint64(0))
			assert.True(t, downloadedBlk.Empty())
			assert.Nil(t, downloadedBlk.Delta)

			downloadedBlk, err = testImporter.GetBlock(uint64(10))
			assert.NoError(t, err)
			assert.Equal(t, downloadedBlk.Round(), uint64(10))
			assert.True(t, downloadedBlk.Empty())
			if ttest.name == followerModeStr {
				// We're not setting the delta yet, but in the future we will
				// assert.NotNil(t, downloadedBlk.Delta)
			} else {
				assert.Nil(t, downloadedBlk.Delta)
			}
			cancel()
		})
	}
}

func TestGetBlockContextCancelled(t *testing.T) {
	tests := []struct {
		name        string
		algodServer *httptest.Server
	}{
		{"archival", NewAlgodServer(GenesisResponder,
			BlockResponder,
			BlockAfterResponder,
			MakeSyncRoundResponder(200))},
		{"follower", NewAlgodServer(GenesisResponder,
			BlockResponder,
			BlockAfterResponder, LedgerStateDeltaResponder,
			MakeSyncRoundResponder(200))},
	}

	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ctx, cancel = context.WithCancel(context.Background())
			testImporter := New()
			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ttest.algodServer.URL)
			_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
			assert.NoError(t, err)
			assert.NotEqual(t, testImporter, nil)

			cancel()
			_, err = testImporter.GetBlock(uint64(10))
			assert.Error(t, err)
		})
	}
}

func TestGetBlockFailure(t *testing.T) {
	tests := []struct {
		name        string
		algodServer *httptest.Server
	}{
		{"archival", NewAlgodServer(GenesisResponder,
			BlockAfterResponder, MakeSyncRoundResponder(200))},
		{"follower", NewAlgodServer(GenesisResponder,
			BlockAfterResponder, LedgerStateDeltaResponder, MakeSyncRoundResponder(200))},
	}
	for _, ttest := range tests {
		t.Run(ttest.name, func(t *testing.T) {
			ctx, cancel = context.WithCancel(context.Background())
			testImporter := New()

			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, ttest.name, ttest.algodServer.URL)
			_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
			assert.NoError(t, err)
			assert.NotEqual(t, testImporter, nil)

			_, err = testImporter.GetBlock(uint64(10))
			assert.Error(t, err)
			cancel()
		})
	}
}

func TestAlgodImporter_ProvideMetrics(t *testing.T) {
	testImporter := &algodImporter{}
	assert.Len(t, testImporter.ProvideMetrics("blah"), 1)
}

func TestGetBlockErrors(t *testing.T) {
	testcases := []struct {
		name                string
		rnd                 uint64
		blockAfterResponder func(string, http.ResponseWriter) bool
		blockResponder      func(string, http.ResponseWriter) bool
		deltaResponder      func(string, http.ResponseWriter) bool
		logs                []string
		err                 string
	}{
		{
			name:                "Cannot get status",
			rnd:                 123,
			blockAfterResponder: MakeJsonResponderSeries("/wait-for-block-after", []int{http.StatusOK, http.StatusNotFound}, []interface{}{models.NodeStatus{}}),
			err:                 fmt.Sprintf("error getting status for round"),
			logs:                []string{"error getting status for round 123", "failed to get block for round 123 "},
		},
		{
			name:                "Cannot get block",
			rnd:                 123,
			blockAfterResponder: BlockAfterResponder,
			blockResponder:      MakeStatusResponder("/v2/blocks/", http.StatusNotFound, ""),
			err:                 fmt.Sprintf("failed to get block"),
			logs:                []string{"error getting block for round 123", "failed to get block for round 123 "},
		},
		{
			name:                "Cannot get delta (node behind)",
			rnd:                 200,
			blockAfterResponder: MakeBlockAfterResponder(models.NodeStatus{LastRound: 50}),
			blockResponder:      BlockResponder,
			deltaResponder:      MakeStatusResponder("/v2/deltas/", http.StatusNotFound, ""),
			err:                 fmt.Sprintf("ledger state delta not found: node round (50) is behind required round (200)"),
			logs:                []string{"ledger state delta not found: node round (50) is behind required round (200)"},
		},
		{
			name:                "Cannot get delta (caught up)",
			rnd:                 200,
			blockAfterResponder: MakeBlockAfterResponder(models.NodeStatus{LastRound: 200}),
			blockResponder:      BlockResponder,
			deltaResponder:      MakeStatusResponder("/v2/deltas/", http.StatusNotFound, ""),
			err:                 fmt.Sprintf("ledger state delta not found: node round (200), required round (200)"),
			logs:                []string{"ledger state delta not found: node round (200), required round (200)"},
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testLogger, hook := test.NewNullLogger()

			// Setup mock algod
			handler := NewAlgodHandler(
				GenesisResponder,
				MakeSyncRoundResponder(200),
				tc.blockAfterResponder,
				tc.blockResponder,
				tc.deltaResponder)
			mockServer := httptest.NewServer(handler)

			// Configure importer to use follow mode and mock server.
			cfg := Config{
				Mode:    followerModeStr,
				NetAddr: mockServer.URL,
			}
			cfgStr, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			pcfg := plugins.MakePluginConfig(string(cfgStr))
			ctx, cancel = context.WithCancel(context.Background())
			testImporter := &algodImporter{}
			_, err = testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), pcfg, testLogger)
			require.NoError(t, err)

			// Run the test
			_, err = testImporter.GetBlock(tc.rnd)
			noError := assert.ErrorContains(t, err, tc.err)

			// Make sure each of the expected log messages are present
			for _, log := range tc.logs {
				found := false
				for _, entry := range hook.AllEntries() {
					found = found || strings.Contains(entry.Message, log)
				}
				noError = noError && assert.True(t, found, "Expected log was not found: '%s'", log)
			}

			// Print logs if there was an error.
			if !noError {
				fmt.Println("An error was detected, printing logs")
				fmt.Println("------------------------------------")
				for _, entry := range hook.AllEntries() {
					fmt.Printf(" %s\n", entry.Message)
				}
				fmt.Println("------------------------------------")
			}
		})
	}
}
