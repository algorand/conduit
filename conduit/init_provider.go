package conduit

import (
	"github.com/algorand/conduit/conduit/telemetry"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

// PipelineInitProvider algod based init provider
type PipelineInitProvider struct {
	currentRound   *sdk.Round
	genesis        *sdk.Genesis
	telemetryState *telemetry.TelemetryState
}

// MakePipelineInitProvider constructs an init provider.
func MakePipelineInitProvider(currentRound *sdk.Round, genesis *sdk.Genesis, state *telemetry.TelemetryState) *PipelineInitProvider {
	return &PipelineInitProvider{
		currentRound:   currentRound,
		genesis:        genesis,
		telemetryState: state,
	}
}

// SetGenesis updates the genesis block in the init provider
func (a *PipelineInitProvider) SetGenesis(genesis *sdk.Genesis) {
	a.genesis = genesis
}

// GetGenesis produces genesis pointer
func (a *PipelineInitProvider) GetGenesis() *sdk.Genesis {
	return a.genesis
}

// NextDBRound provides next database round
func (a *PipelineInitProvider) NextDBRound() sdk.Round {
	return *a.currentRound
}

// SetTelemetryState updates the telemetry state in the init provider
func (a *PipelineInitProvider) SetTelemetryState(state *telemetry.TelemetryState) {
	a.telemetryState = state
}

// GetTelemetryState gets the telemetry state in the init provider
func (a *PipelineInitProvider) GetTelemetryState() *telemetry.TelemetryState {
	return a.telemetryState
}
