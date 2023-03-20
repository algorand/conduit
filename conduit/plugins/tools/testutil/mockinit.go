package testutil

import sdk "github.com/algorand/go-algorand-sdk/v2/types"

// MockInitProvider mock an init provider
type MockInitProvider struct {
	CurrentRound *sdk.Round
	Genesis      *sdk.Genesis
}

// GetGenesis produces genesis pointer
func (m *MockInitProvider) GetGenesis() *sdk.Genesis {
	return m.Genesis
}

// SetGenesis updates the genesis block in the init provider
func (m *MockInitProvider) SetGenesis(genesis *sdk.Genesis) {
	m.Genesis = genesis
}

// NextDBRound provides next database round
func (m *MockInitProvider) NextDBRound() sdk.Round {
	return *m.CurrentRound
}

// MockedInitProvider returns an InitProvider for testing
func MockedInitProvider(round *sdk.Round) *MockInitProvider {
	return &MockInitProvider{
		CurrentRound: round,
		Genesis:      &sdk.Genesis{},
	}
}
