package filterprocessor

import (
	"container/ring"
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/algorand/conduit/conduit"
	"github.com/algorand/conduit/conduit/data"
	"github.com/algorand/conduit/conduit/plugins"
)

func emptyBlockData() data.BlockData {
	return data.BlockData{
		BlockHeader: sdk.BlockHeader{},
		Payset:      nil,
		Delta:       nil,
		Certificate: nil,
	}
}

type typeRing struct {
	ring *ring.Ring
}

func makeTypeRing(typeRoundRobin ...sdk.TxType) *typeRing {
	r := ring.New(len(typeRoundRobin))
	for _, t := range typeRoundRobin {
		r.Value = t
		r = r.Next()
	}
	return &typeRing{r}
}

func (tr *typeRing) next() sdk.TxType {
	// the first next() returns the first value
	ret := tr.ring.Value.(sdk.TxType)
	tr.ring = tr.ring.Next()
	return ret
}

func TestTypeRing1(t *testing.T) {
	types := makeTypeRing(sdk.PaymentTx, sdk.ApplicationCallTx)
	assert.Equal(t, sdk.PaymentTx, types.next())
	assert.Equal(t, sdk.ApplicationCallTx, types.next())
	assert.Equal(t, sdk.PaymentTx, types.next())
}

func TestTypeRing2(t *testing.T) {
	types := makeTypeRing(sdk.PaymentTx)
	assert.Equal(t, sdk.PaymentTx, types.next())
	assert.Equal(t, sdk.PaymentTx, types.next())
}

// appendTxns creates and adds top level transactions to the block
func appendTxns(bd data.BlockData, numTxn int, note []byte, typeRoundRobin ...sdk.TxType) data.BlockData {
	types := makeTypeRing(typeRoundRobin...)

	var payset []sdk.SignedTxnInBlock
	for i := 0; i < numTxn; i++ {
		txn := sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						Type: types.next(),
						Header: sdk.Header{
							Note: note,
						},
					},
				},
			},
		}
		payset = append(payset, txn)
	}

	bd.Payset = payset
	return bd
}

// appendInnerTxns creates and adds inner transactions to each top level transactions in the block
func appendInnerTxns(bd data.BlockData, numInner int, typeRoundRobin ...sdk.TxType) data.BlockData {
	if numInner == 0 {
		return bd
	}
	types := makeTypeRing(typeRoundRobin...)
	var payset []sdk.SignedTxnInBlock
	for _, txn := range bd.Payset {
		for i := 0; i < numInner; i++ {
			innerTxn := sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						Type: types.next(),
					},
				},
			}
			txn.EvalDelta.InnerTxns = append(txn.EvalDelta.InnerTxns, innerTxn)
		}

		payset = append(payset, txn)
	}

	bd.Payset = payset
	return bd
}

func BenchmarkProcess(b *testing.B) {
	var addr sdk.Address
	addr[0] = 0x01

	// special rules for this tests block...
	testBlock := func(numInner int) data.BlockData {
		bd := emptyBlockData()
		// One transaction, with variable inner txns
		bd = appendTxns(bd, 1, nil, sdk.ApplicationCallTx)
		bd = appendInnerTxns(bd, numInner, sdk.PaymentTx)

		// signer and group in first txn
		txn := bd.Payset[0]
		txn.SignedTxn.AuthAddr = addr
		txn.SignedTxn.Txn.Header.Group = sdk.Digest{1}
		// If there are inner transactions... match signer in inner rather than top level
		if len(txn.EvalDelta.InnerTxns) > 0 {
			txn.AuthAddr = sdk.ZeroAddress
			txn.EvalDelta.InnerTxns[len(txn.EvalDelta.InnerTxns)-1].SignedTxn.AuthAddr = addr
		}
		bd.Payset[0] = txn

		// Add a 2nd txn to for "omitGroupTxns" test
		bd.Payset = append(bd.Payset, sdk.SignedTxnInBlock{
			SignedTxnWithAD: sdk.SignedTxnWithAD{
				SignedTxn: sdk.SignedTxn{
					Txn: sdk.Transaction{
						Header: sdk.Header{
							Group: sdk.Digest{1},
						},
					},
				},
			},
		})
		return bd
	}

	var table = []struct {
		numInner      int
		outputlen     int
		omitGroupTxns bool
	}{
		{numInner: 0, outputlen: 1, omitGroupTxns: true},
		{numInner: 10, outputlen: 1, omitGroupTxns: true},
		{numInner: 100, outputlen: 1, omitGroupTxns: true},
		{numInner: 0, outputlen: 2, omitGroupTxns: false},
		{numInner: 10, outputlen: 2, omitGroupTxns: false},
		{numInner: 100, outputlen: 2, omitGroupTxns: false},
	}
	for _, v := range table {
		b.Run(fmt.Sprintf("inner_txn_count_%d_omitGrouptxns_%t", v.numInner, v.omitGroupTxns), func(b *testing.B) {
			bd := testBlock(v.numInner)
			cfgStr := fmt.Sprintf(`search-inner: true
omit-group-transactions: %t
filters:
  - all:
    - tag: sgnr
      expression-type: equal
      expression: "%s"`, v.omitGroupTxns, addr.String())

			fp := &FilterProcessor{}
			err := fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(cfgStr), logrus.New())
			assert.NoError(b, err)

			// sanity test Process
			{
				out, err := fp.Process(bd)
				require.NoError(b, err)
				require.Len(b, out.Payset, v.outputlen)
			}

			// Ignore the setup cost above.
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				fp.Process(bd)
			}
		})
	}
}

type builder struct {
	all  map[string][]SubConfig
	any  map[string][]SubConfig
	none map[string][]SubConfig
}

func (c *builder) add(t string, sc SubConfig) {
	var m map[string][]SubConfig
	switch t {
	case "all":
		if c.all == nil {
			c.all = make(map[string][]SubConfig)
		}
		m = c.all
	case "none":
		if c.none == nil {
			c.none = make(map[string][]SubConfig)
		}
		m = c.none
	case "any":
		if c.any == nil {
			c.any = make(map[string][]SubConfig)
		}
		m = c.any
	default:
		panic(fmt.Sprintf("unknown type: %s", t))
	}

	m[t] = append(m[t], sc)
}

func (c *builder) build() *Config {
	ret := &Config{
		Filters: []map[string][]SubConfig{},
	}

	if c.all != nil {
		ret.Filters = append(ret.Filters, c.all)
	}
	if c.any != nil {
		ret.Filters = append(ret.Filters, c.any)
	}
	if c.none != nil {
		ret.Filters = append(ret.Filters, c.none)
	}
	return ret
}

func BenchmarkProcess2(b *testing.B) {
	// Block:
	// * 25000 top level transactions
	//   * 5000x app call, payment, asset cfg, keyreg, asset xfer
	// * 25000 inner payment transactions
	bd := emptyBlockData()
	// 5000 app calls
	bd = appendTxns(bd, 5000, []byte("note data to match on"), sdk.ApplicationCallTx)
	// Add an inner payment to each of these
	bd = appendInnerTxns(bd, 5, sdk.PaymentTx)
	// 20000 payments, asset, keyreg, and asset transfers
	bd = appendTxns(bd, 20000, []byte("payment txn"), sdk.PaymentTx, sdk.AssetConfigTx, sdk.KeyRegistrationTx, sdk.AssetTransferTx)

	cfgWithFilters := func(numEqual int, eqExp string, numRegex int, reExp string, numNum int, numExp string) *Config {
		bldr := &builder{}
		for i := 0; i < numEqual; i++ {
			sc := SubConfig{
				FilterTag:      "sgnr",
				ExpressionType: "equal",
				Expression:     eqExp,
			}
			bldr.add("all", sc)
		}
		for i := 0; i < numRegex; i++ {
			sc := SubConfig{
				FilterTag:      "txn.note",
				ExpressionType: "regex",
				Expression:     reExp,
			}
			bldr.add("all", sc)
		}
		for i := 0; i < numNum; i++ {
			sc := SubConfig{
				FilterTag:      "txn.fee",
				ExpressionType: "equal",
				Expression:     numExp,
			}
			bldr.add("all", sc)
		}

		return bldr.build()
	}

	var table = []struct {
		name   string
		config string
		numTxn int
		numEq  int
		numRe  int
		numNum int
	}{
		// equal filter
		{
			name:   "equal",
			numTxn: 25000,
			numEq:  1000,
		},
		{
			name:   "equal",
			numTxn: 1,
			numEq:  1000,
		},
		{
			name:   "equal",
			numTxn: 25000,
			numEq:  1,
		},
		{
			name:   "equal",
			numTxn: 1,
			numEq:  1,
		},

		// regex filter
		{
			name:   "regex",
			numTxn: 25000,
			numRe:  1000,
		},
		{
			name:   "regex",
			numTxn: 1,
			numRe:  1000,
		},
		{
			name:   "regex",
			numTxn: 25000,
			numRe:  1,
		},
		{
			name:   "regex",
			numTxn: 1,
			numRe:  1,
		},

		// numeric filter
		{
			name:   "equal",
			numTxn: 25000,
			numNum: 1000,
		},
		{
			name:   "equal",
			numTxn: 25000,
			numNum: 1,
		},
		{
			name:   "equal",
			numTxn: 1,
			numNum: 1000,
		},
		{
			name:   "equal",
			numTxn: 1,
			numNum: 1,
		},
	}
	for _, v := range table {
		b.Run(fmt.Sprintf("txn_%d_equal_%d_regex_%d_numeric_%d", v.numTxn, v.numEq, v.numRe, v.numNum), func(b *testing.B) {
			cfg := cfgWithFilters(v.numEq, "addr", v.numRe, "note", v.numNum, "1000")
			cfgStr, err := yaml.Marshal(cfg)
			require.NoError(b, err)

			fp := &FilterProcessor{}
			err = fp.Init(context.Background(), &conduit.PipelineInitProvider{}, plugins.MakePluginConfig(string(cfgStr)), logrus.New())
			assert.NoError(b, err)

			// Ignore the setup cost above.
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				fp.Process(bd)
			}
		})
	}
}
