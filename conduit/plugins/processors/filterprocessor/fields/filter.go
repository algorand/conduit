package fields

import (
	"fmt"

	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// Operation an operation like "any" or "all" for boolean logic
type Operation string

const anyFieldOperation Operation = "any"
const allFieldOperation Operation = "all"
const noneFieldOperation Operation = "none"

// ValidFieldOperation returns true if the input is a valid operation
func ValidFieldOperation(input string) bool {
	if input != string(anyFieldOperation) && input != string(allFieldOperation) && input != string(noneFieldOperation) {
		return false
	}

	return true
}

// Filter an object that combines field searches with a boolean operator
type Filter struct {
	Op        Operation
	Searchers []*Searcher
	OmitGroup bool
}

func (f Filter) matches(txn *sdk.SignedTxnWithAD) (bool, error) {
	numMatches := 0
	for _, fs := range f.Searchers {
		b, err := fs.search(txn)
		if err != nil {
			return false, err
		}
		if b {
			numMatches++
		}
	}

	switch f.Op {
	case noneFieldOperation:
		return numMatches == 0, nil
	case anyFieldOperation:
		return numMatches > 0, nil
	case allFieldOperation:
		return numMatches == len(f.Searchers), nil
	default:
		return false, fmt.Errorf("unknown operation: %s", f.Op)
	}
}

// SearchAndFilter searches through the block data and applies the operation to the results
func (f Filter) SearchAndFilter(payset []sdk.SignedTxnInBlock) ([]sdk.SignedTxnInBlock, error) {
	var result []sdk.SignedTxnInBlock
	firstGroupIdx := 0
	for i := 0; i < len(payset); i++ {
		if payset[firstGroupIdx].Txn.Group != payset[i].Txn.Group {
			firstGroupIdx = i
		}
		match, err := f.matches(&payset[i].SignedTxnWithAD)
		if err != nil {
			return nil, err
		}
		if match {
			// if txn.Group is set and omit group is false
			if payset[i].Txn.Group != (sdk.Digest{}) && !f.OmitGroup {
				j := firstGroupIdx
				// append all txns with same group ID
				for ; j < len(payset) && payset[j].Txn.Group == payset[firstGroupIdx].Txn.Group; j++ {
					result = append(result, payset[j])
				}
				// skip txns that are already added, set i to the index of the last added txn
				i = j - 1
			} else {
				result = append(result, payset[i])
			}
		}
	}

	return result, nil
}
