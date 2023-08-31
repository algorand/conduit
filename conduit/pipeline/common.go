package pipeline

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/algorand/conduit/conduit/data"
)

// HandlePanic function to log panics in a common way
func HandlePanic(logger *log.Logger) {
	if r := recover(); r != nil {
		logger.Panicf("conduit pipeline experienced a panic: %v", r)
	}
}

type empty struct{}

type pluginInput interface {
	uint64 | data.BlockData | string
}

type pluginOutput interface {
	pluginInput | empty
}

// retries is a wrapper for retrying a function call f() with a cancellation context,
// a delay and a max retry count. It attempts to call the wrapped function at least once
// and only after the first attempt will pay attention to a context cancellation.
// This can allow the pipeline to receive a cancellation and guarantee attempting to finish
// the round with at least one attempt for each pipeline component.
// - Retry behavior is configured by p.cfg.retryCount.
//   - when 0, the function will retry forever or until the context is canceled
//   - when > 0, the function will retry p.cfg.retryCount times before giving up
//
// - Upon success:
//   - a nil error is returned even if there were intermediate failures
//   - the returned duration dur measures the time spent in the call to f() that succeeded
//
// - Upon failure:
//   - the return value y is the zero value of type Y and a non-nil error is returned
//   - when p.cfg.retryCount > 0, the error will be a join of all the errors encountered during the retries
//   - when p.cfg.retryCount == 0, the error will be the last error encountered
//   - the returned duration dur is the total time spent in the function, including retries
func retries[X pluginInput, Y pluginOutput](f func(x X) (Y, error), x X, p *pipelineImpl, msg string) (y Y, dur time.Duration, err error) {
	start := time.Now()

	for i := uint64(0); p.cfg.RetryCount == 0 || i <= p.cfg.RetryCount; i++ {
		// the first time through, we don't sleep or mind ctx's done signal
		if i > 0 {
			select {
			case <-p.ctx.Done():
				dur = time.Since(start)
				err = fmt.Errorf("%s: retry number %d/%d with err: %w. Done signal received: %w", msg, i, p.cfg.RetryCount, err, context.Cause(p.ctx))
				return
			default:
				time.Sleep(p.cfg.RetryDelay)
			}
		}
		opStart := time.Now()
		y, err = f(x)
		if err == nil {
			dur = time.Since(opStart)
			return
		}
		p.logger.Infof("%s: retry number %d/%d with err: %v", msg, i, p.cfg.RetryCount, err)
	}

	dur = time.Since(start)
	err = fmt.Errorf("%s: giving up after %d retries: %w", msg, p.cfg.RetryCount, err)
	return
}

// retriesNoOutput applies the same logic as Retries, but for functions that return no output.
func retriesNoOutput[X pluginInput](f func(x X) error, a X, p *pipelineImpl, msg string) (time.Duration, error) {
	_, d, err := retries(func(x X) (empty, error) {
		return empty{}, f(x)
	}, a, p, msg)
	return d, err
}
