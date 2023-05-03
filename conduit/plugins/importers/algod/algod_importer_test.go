package algodimporter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
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
	ts := NewAlgodServer(GenesisResponder, MakeSyncRoundResponder(http.StatusOK), BlockAfterResponder)
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
		name      string
		responder func(string, http.ResponseWriter) bool
	}{
		{
			name:      "archival",
			responder: MakeSyncRoundResponder(http.StatusNotFound),
		},
		{
			name:      "follower",
			responder: MakeSyncRoundResponder(http.StatusOK),
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ts := NewAlgodServer(GenesisResponder, tc.responder, BlockAfterResponder)
			testImporter := New()
			cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, tc.name, ts.URL)
			_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
			assert.NoError(t, err)
			assert.NotEqual(t, testImporter, nil)
			testImporter.Close()
		})
	}
}

func Test_checkRounds(t *testing.T) {
	type args struct {
		catchpointRound uint64
		nodeRound       uint64
		targetRound     uint64
	}
	tests := []struct {
		name         string
		args         args
		want         bool
		wantErr      assert.ErrorAssertionFunc
		wantLogLevel logrus.Level
		wantLogMsg   string
	}{
		{
			name: "Skip catchpoint",
			args: args{
				catchpointRound: 1000,
				nodeRound:       1001,
				targetRound:     1002,
			},
			want:         false,
			wantErr:      assert.NoError,
			wantLogLevel: logrus.InfoLevel,
			wantLogMsg:   "No catchup required. Node round 1001, target round 1002, catchpoint round 1000.",
		},
		{
			name: "Catchup requested.",
			args: args{
				catchpointRound: 1002,
				nodeRound:       1001,
				targetRound:     1003,
			},
			want:         true,
			wantErr:      assert.NoError,
			wantLogLevel: logrus.InfoLevel,
			wantLogMsg:   "Catchup requested. Node round 1001, target round 1003, catchpoint round 1002.",
		},
		{
			name: "Catchup required. Success.",
			args: args{
				catchpointRound: 1000,
				nodeRound:       5000,
				targetRound:     1002,
			},
			want:         true,
			wantErr:      assert.NoError,
			wantLogLevel: logrus.InfoLevel,
			wantLogMsg:   "Catchup required, node round ahead of target round. Node round 5000, target round 1002, catchpoint round 1000.",
		},
		{
			name: "Catchup required. Error.",
			args: args{
				catchpointRound: 6000,
				nodeRound:       5000,
				targetRound:     1002,
			},
			want:         false,
			wantErr:      assert.Error,
			wantLogLevel: logrus.ErrorLevel,
			wantLogMsg:   "Catchup required but no valid catchpoint available, node round 5000 and catchpoint round 6000 are ahead of target round 1002.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testLogger, hook := test.NewNullLogger()
			got, err := checkRounds(testLogger, tt.args.catchpointRound, tt.args.nodeRound, tt.args.targetRound)

			// Write 1 line to the log.
			require.Len(t, hook.Entries, 1)
			require.Equal(t, tt.wantLogLevel, hook.Entries[0].Level)
			require.Equal(t, tt.wantLogMsg, hook.Entries[0].Message)

			// Check the error
			if !tt.wantErr(t, err, fmt.Sprintf("checkRounds(-, %v, %v, %v)", tt.args.catchpointRound, tt.args.nodeRound, tt.args.targetRound)) {
				return
			}

			// Check return values
			assert.Equalf(t, tt.want, got, "checkRounds(-, %v, %v, %v)", tt.args.catchpointRound, tt.args.nodeRound, tt.args.targetRound)

		})
	}
}

func TestInitCatchup(t *testing.T) {
	tests := []struct {
		name        string
		catchpoint  string
		targetRound sdk.Round
		auto        bool
		algodServer *httptest.Server
		err         string
		logs        []string
	}{
		{
			name:        "sync round failure",
			targetRound: 1,
			algodServer: NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(http.StatusBadRequest)),
			err:  "received unexpected error setting sync round (1): HTTP 400",
			logs: []string{}},
		{
			name:       "catchpoint parse failure",
			catchpoint: "notvalid",
			algodServer: NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(http.StatusOK)),
			err:  "unable to parse catchpoint, invalid format",
			logs: []string{}},
		{
			name:       "invalid catchpoint round uint parsing error",
			catchpoint: "abcd#abcd",
			algodServer: NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(http.StatusOK)),
			err:  "invalid syntax",
			logs: []string{}},
		{
			name:       "node status failure",
			catchpoint: "1234#abcd",
			algodServer: NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(http.StatusOK),
				MakeStatusResponder("/v2/status", http.StatusBadRequest, "")),
			err:  "received unexpected error failed to get node status: HTTP 400",
			logs: []string{}},
		{
			name:        "catchpoint round before node round skips fast catchup",
			catchpoint:  "1234#abcd",
			targetRound: 1235,
			algodServer: NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(http.StatusOK),
				MakeNodeStatusResponder(models.NodeStatus{LastRound: 1235})),
			logs: []string{"No catchup required. Node round 1235, target round 1235, catchpoint round 1234."},
		}, {
			name:        "start catchpoint catchup failure",
			catchpoint:  "1236#abcd",
			targetRound: 1240,
			algodServer: NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(http.StatusOK),
				MakeNodeStatusResponder(models.NodeStatus{LastRound: 1235}),
				MakeStatusResponder("/v2/catchup/", http.StatusBadRequest, "")),
			err:  "POST /v2/catchup/1236#abcd received unexpected error: HTTP 400",
			logs: []string{},
		},
		{
			name:        "monitor catchup node status failure",
			catchpoint:  "1236#abcd",
			targetRound: 1239,
			algodServer: NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(http.StatusOK),
				MakeJsonResponderSeries("/v2/status", []int{http.StatusOK, http.StatusBadRequest}, []interface{}{models.NodeStatus{LastRound: 1235}}),
				MakeStatusResponder("/v2/catchup/", http.StatusOK, "")),
			err:  "received unexpected error getting node status: HTTP 400",
			logs: []string{},
		}, {
			name:        "monitor catchup success",
			catchpoint:  "1236#abcd",
			targetRound: 1240,
			auto:        true, // ignored
			algodServer: NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(http.StatusOK),
				MakeJsonResponderSeries("/v2/status", []int{http.StatusOK}, []interface{}{
					models.NodeStatus{LastRound: 1235},
					models.NodeStatus{Catchpoint: "1236#abcd", CatchpointProcessedAccounts: 1, CatchpointTotalAccounts: 1},
					models.NodeStatus{Catchpoint: "1236#abcd", CatchpointVerifiedAccounts: 1, CatchpointTotalAccounts: 1},
					models.NodeStatus{Catchpoint: "1236#abcd", CatchpointAcquiredBlocks: 1, CatchpointTotalBlocks: 1},
					models.NodeStatus{Catchpoint: "1236#abcd"},
					models.NodeStatus{LastRound: 1236},
				}),
				MakeStatusResponder("/v2/catchup/", http.StatusOK, "")),
			logs: []string{
				"catchup phase Processed Accounts: 1 / 1",
				"catchup phase Verified Accounts: 1 / 1",
				"catchup phase Acquired Blocks: 1 / 1",
				"catchup phase Verified Blocks",
			},
		}, {
			name:       "auto catchup used (even if the mocking isn't setup for it)",
			catchpoint: "",
			auto:       true,
			algodServer: NewAlgodServer(
				GenesisResponder,
			),
			err: "failed to lookup catchpoint label list",
		}, {
			name:        "wait for node to catchup error",
			targetRound: 1240,
			catchpoint:  "1236#abcd",
			algodServer: NewAlgodServer(
				GenesisResponder,
				MakeSyncRoundResponder(http.StatusOK),
				MakeJsonResponderSeries("/v2/status", []int{http.StatusOK, http.StatusOK, http.StatusBadRequest}, []interface{}{models.NodeStatus{LastRound: 1235}}),
				MakeStatusResponder("/v2/catchup/", http.StatusOK, "")),
			err:  "received unexpected error (StatusAfterBlock) waiting for node to catchup: HTTP 400",
			logs: []string{},
		},
	}
	for _, ttest := range tests {
		ttest := ttest
		t.Run(ttest.name, func(t *testing.T) {
			t.Parallel()
			testLogger, hook := test.NewNullLogger()
			testImporter := &algodImporter{}
			cfg := Config{
				Mode:    "follower",
				NetAddr: ttest.algodServer.URL,
				CatchupConfig: CatchupParams{
					Catchpoint: ttest.catchpoint,
					Auto:       ttest.auto,
				},
			}
			cfgStr, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			_, err = testImporter.Init(ctx, conduit.MakePipelineInitProvider(&ttest.targetRound, nil), plugins.MakePluginConfig(string(cfgStr)), testLogger)
			if ttest.err != "" {
				require.ErrorContains(t, err, ttest.err)
			} else {
				require.NoError(t, err)
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
	url := ".0.0.0.0.0.0.0:1234"
	testImporter := New()
	cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, "follower", url)
	_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
	assert.ErrorContains(t, err, "parse")
}

func TestInitModeFailure(t *testing.T) {
	name := "foobar"
	ts := NewAlgodServer(GenesisResponder)
	testImporter := New()
	cfgStr := fmt.Sprintf(`---
mode: %s
netaddr: %s
`, name, ts.URL)
	_, err := testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(cfgStr), logger)
	assert.EqualError(t, err, fmt.Sprintf("algod importer was set to a mode (%s) that wasn't supported", name))
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
	ts := NewAlgodServer(GenesisResponder, MakeSyncRoundResponder(http.StatusNotFound), BlockAfterResponder)
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
		mode        string
		algodServer *httptest.Server
	}{
		{
			name: "default",
			mode: "",
			algodServer: NewAlgodServer(GenesisResponder,
				BlockResponder,
				BlockAfterResponder,
				MakeSyncRoundResponder(http.StatusNotFound))},
		{
			name: "archival",
			mode: "archival",
			algodServer: NewAlgodServer(GenesisResponder,
				BlockResponder,
				BlockAfterResponder,
				MakeSyncRoundResponder(http.StatusNotFound))},
		{
			name: "follower",
			mode: "follower",
			algodServer: NewAlgodServer(GenesisResponder,
				BlockResponder,
				BlockAfterResponder, LedgerStateDeltaResponder, MakeSyncRoundResponder(http.StatusOK)),
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := Config{
				Mode:    tc.mode,
				NetAddr: tc.algodServer.URL,
			}
			cfgStr, err := yaml.Marshal(cfg)
			require.NoError(t, err)

			ctx, cancel = context.WithCancel(context.Background())
			defer cancel()
			testImporter := &algodImporter{}

			_, err = testImporter.Init(ctx, conduit.MakePipelineInitProvider(&pRound, nil), plugins.MakePluginConfig(string(cfgStr)), logger)
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
			// Delta only set in follower mode
			if tc.name == followerModeStr {
				assert.NotNil(t, downloadedBlk.Delta)
			} else {
				assert.Nil(t, downloadedBlk.Delta)
			}
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
			MakeSyncRoundResponder(http.StatusNotFound))},
		{"follower", NewAlgodServer(GenesisResponder,
			BlockResponder,
			BlockAfterResponder, LedgerStateDeltaResponder,
			MakeSyncRoundResponder(http.StatusOK))},
	}

	for _, ttest := range tests {
		ttest := ttest
		t.Run(ttest.name, func(t *testing.T) {
			// this didn't work...
			//t.Parallel()
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
			BlockAfterResponder, MakeSyncRoundResponder(http.StatusNotFound))},
		{"follower", NewAlgodServer(GenesisResponder,
			BlockAfterResponder, LedgerStateDeltaResponder, MakeSyncRoundResponder(http.StatusOK))},
	}
	for _, ttest := range tests {
		ttest := ttest
		t.Run(ttest.name, func(t *testing.T) {
			t.Parallel()
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
				MakeSyncRoundResponder(http.StatusOK),
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

func TestGetMissingCatchpointLabel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "1000#abcd\n1100#abcd\n1200#abcd")
	}))
	defer ts.Close()
	label, err := getMissingCatchpointLabel(ts.URL, 1100)
	require.NoError(t, err)
	// closest without going over
	require.Equal(t, "1100#abcd", label)
}

func TestGetMissingCatchpointLabelError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "")
	}))
	defer ts.Close()
	_, err := getMissingCatchpointLabel(ts.URL, 1100)
	require.ErrorContains(t, err, "no catchpoint label found for round 1100 at:")
}
