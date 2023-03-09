package conduit

// CriticalError an error that causes the entire conduit pipeline to
// stop
type CriticalError struct{}

func (e *CriticalError) Error() string {
	return "critical error occurred"
}
