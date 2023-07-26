package pipeline

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/algorand/conduit/conduit/data"
	log "github.com/sirupsen/logrus"
)

// HandlePanic function to log panics in a common way
func HandlePanic(logger *log.Logger) {
	if r := recover(); r != nil {
		logger.Panicf("conduit pipeline experienced a panic: %v", r)
	}
}

type empty struct{}

type pluginInput interface {
	uint64 | data.BlockData | string | empty
}

func Retries[X, Y pluginInput](f func(x X) (Y, error), x X, p *pipelineImpl, msg string) (y Y, dur time.Duration, err error) {
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
		y2, err2 := f(x)
		if err2 == nil {
			return y2, time.Since(opStart), nil
		}

		p.logger.Infof("%s: retry number %d/%d with err: %v", msg, i, p.cfg.RetryCount, err2)
		if p.cfg.RetryCount > 0 {
			err = errors.Join(err, err2)
		} else {
			// in the case of infinite retries, only keep the last error
			err = err2
		}
	}

	dur = time.Since(start)
	err = fmt.Errorf("%s: giving up after %d retries: %w", msg, p.cfg.RetryCount, err)
	return
}

func RetriesNoOutput[X pluginInput](f func(a X) error, a X, p *pipelineImpl, msg string) (time.Duration, error) {
	_, d, err := Retries(func(a X) (empty, error) {
		return empty{}, f(a)
	}, a, p, msg)
	return d, err
}
