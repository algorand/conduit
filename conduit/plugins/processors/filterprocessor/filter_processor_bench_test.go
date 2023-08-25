package filterprocessor

import (
	"container/ring"
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

			//bd := blockData(addr, v.numInner)
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
