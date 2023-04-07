package pipeline

import "fmt"

// ErrOverrideConflict is used when there are two different round overrides detected.
type ErrOverrideConflict struct {
	firstName   string
	firstRound  uint64
	secondName  string
	secondRound uint64
}

// makeErrOverrideConflict creates a new ErrOverrideConflict.
func makeErrOverrideConflict(firstName string, firstRound uint64, secondName string, secondRound uint64) error {
	return ErrOverrideConflict{
		firstName:   firstName,
		firstRound:  firstRound,
		secondName:  secondName,
		secondRound: secondRound,
	}
}

// Error implements the error interface.
func (e ErrOverrideConflict) Error() string {
	return fmt.Sprintf("inconsistent round overrides detected: %s (%d), %s (%d)", e.firstName, e.firstRound, e.secondName, e.secondRound)
}
