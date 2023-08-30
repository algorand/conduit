package pipeline

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/algorand/conduit/conduit/data"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// TestRetries tests the retry logic
func TestRetries(t *testing.T) {
	errSentinelCause := errors.New("succeed after has failed")

	succeedAfterFactory := func(succeedAfter uint64, never bool) func(uint64) (uint64, error) {
		tries := uint64(0)

		return func(x uint64) (uint64, error) {
			if tries >= succeedAfter && !never {
				// ensure not to return the zero value on success
				return tries + 1, nil
			}
			tries++
			return 0, fmt.Errorf("%w: tries=%d", errSentinelCause, tries-1)
		}
	}

	succeedAfterFactoryNoOutput := func(succeedAfter uint64, never bool) func(uint64) error {
		tries := uint64(0)

		return func(x uint64) error {
			if tries >= succeedAfter && !never {
				return nil
			}
			tries++
			return fmt.Errorf("%w: tries=%d", errSentinelCause, tries-1)
		}
	}

	cases := []struct {
		name         string
		retryCount   uint64
		succeedAfter uint64
		neverSucceed bool // neverSucceed trumps succeedAfter
	}{
		{
			name:         "retry forever succeeds after 0",
			retryCount:   0,
			succeedAfter: 0,
			neverSucceed: false,
		},
		{
			name:         "retry forever succeeds after 1",
			retryCount:   0,
			succeedAfter: 1,
			neverSucceed: false,
		},
		{
			name:         "retry forever succeeds after 7",
			retryCount:   0,
			succeedAfter: 7,
			neverSucceed: false,
		},
		{
			name:         "retry 5 succeeds after 0",
			retryCount:   5,
			succeedAfter: 0,
			neverSucceed: false,
		},
		{
			name:         "retry 5 succeeds after 1",
			retryCount:   5,
			succeedAfter: 1,
			neverSucceed: false,
		},
		{
			name:         "retry 5 succeeds after 5",
			retryCount:   5,
			succeedAfter: 5,
			neverSucceed: false,
		},
		{
			name:         "retry 5 succeeds after 7",
			retryCount:   5,
			succeedAfter: 7,
			neverSucceed: false,
		},
		{
			name:         "retry 5 never succeeds",
			retryCount:   5,
			succeedAfter: 0,
			neverSucceed: true,
		},
		{
			name:         "retry foerever never succeeds",
			retryCount:   0,
			succeedAfter: 0,
			neverSucceed: true,
		},
	}

	for _, tc := range cases {
		tc := tc

		// run cases for retries()
		t.Run("retries() "+tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, ccf := context.WithCancelCause(context.Background())
			p := &pipelineImpl{
				ctx:    ctx,
				ccf:    ccf,
				logger: log.New(),
				cfg: &data.Config{
					RetryCount: tc.retryCount,
					RetryDelay: 1 * time.Millisecond,
				},
			}
			succeedAfter := succeedAfterFactory(tc.succeedAfter, tc.neverSucceed)

			if tc.retryCount == 0 && tc.neverSucceed {
				// avoid infinite loop by cancelling the context

				yChan := make(chan uint64)
				errChan := make(chan error)
				go func() {
					y, _, err := retries(succeedAfter, 0, p, "test")
					yChan <- y
					errChan <- err
				}()
				time.Sleep(5 * time.Millisecond)
				errTestCancelled := errors.New("test cancelled")
				go func() {
					ccf(errTestCancelled)
				}()
				y := <-yChan
				err := <-errChan
				require.ErrorIs(t, err, errTestCancelled, tc.name)
				require.ErrorIs(t, err, errSentinelCause, tc.name)
				require.Zero(t, y, tc.name)
				return
			}

			y, _, err := retries(succeedAfter, 0, p, "test")
			if tc.retryCount == 0 { // WLOG tc.neverSucceed == false
				require.NoError(t, err, tc.name)

				// note we subtract 1 from y below because succeedAfter has added 1 to its output
				// to disambiguate with the zero value which occurs on failure
				require.Equal(t, tc.succeedAfter, y-1, tc.name)
			} else { // retryCount > 0 so doesn't retry forever
				if tc.neverSucceed || tc.succeedAfter > tc.retryCount {
					require.ErrorContains(t, err, fmt.Sprintf("%d", tc.retryCount), tc.name)
					require.ErrorIs(t, err, errSentinelCause, tc.name)
					require.Zero(t, y, tc.name)
				} else { // !tc.neverSucceed && succeedAfter <= retryCount
					require.NoError(t, err, tc.name)
					require.Equal(t, tc.succeedAfter, y-1, tc.name)
				}
			}
		})

		// run cases for retriesNoOutput()
		t.Run("retriesNoOutput() "+tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, ccf := context.WithCancelCause(context.Background())
			p := &pipelineImpl{
				ctx:    ctx,
				ccf:    ccf,
				logger: log.New(),
				cfg: &data.Config{
					RetryCount: tc.retryCount,
					RetryDelay: 1 * time.Millisecond,
				},
			}
			succeedAfterNoOutput := succeedAfterFactoryNoOutput(tc.succeedAfter, tc.neverSucceed)

			if tc.retryCount == 0 && tc.neverSucceed {
				// avoid infinite loop by cancelling the context

				errChan := make(chan error)
				go func() {
					_, err := retriesNoOutput(succeedAfterNoOutput, 0, p, "test")
					errChan <- err
				}()
				time.Sleep(5 * time.Millisecond)
				errTestCancelled := errors.New("test cancelled")
				go func() {
					ccf(errTestCancelled)
				}()
				err := <-errChan
				require.ErrorIs(t, err, errTestCancelled, tc.name)
				require.ErrorIs(t, err, errSentinelCause, tc.name)
				return
			}

			_, err := retriesNoOutput(succeedAfterNoOutput, 0, p, "test")
			if tc.retryCount == 0 { // WLOG tc.neverSucceed == false
				require.NoError(t, err, tc.name)
			} else { // retryCount > 0 so doesn't retry forever
				if tc.neverSucceed || tc.succeedAfter > tc.retryCount {
					require.ErrorContains(t, err, fmt.Sprintf("%d", tc.retryCount), tc.name)
					require.ErrorIs(t, err, errSentinelCause, tc.name)
				} else { // !tc.neverSucceed && succeedAfter <= retryCount
					require.NoError(t, err, tc.name)
				}
			}
		})
	}
}
