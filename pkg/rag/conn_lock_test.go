package rag

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryWhileLockedWaitsForTheHolderToRelease(t *testing.T) {
	calls := 0
	err := retryWhileLocked(context.Background(), 5*time.Second, func() error {
		calls++
		if calls < 3 {
			return errors.New("sidecar error -32000: ledger: Database already open. Cannot acquire lock.")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Errorf("err = %v after %d calls, want success on the third", err, calls)
	}
}

func TestRetryWhileLockedGivesUpAndOnlyRetriesLockErrors(t *testing.T) {
	locked := errors.New("ledger: Database already open")
	start := time.Now()
	if err := retryWhileLocked(context.Background(), 300*time.Millisecond, func() error { return locked }); !errors.Is(err, locked) {
		t.Errorf("err = %v, want the lock error after the wait", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("kept retrying past the wait")
	}

	calls := 0
	other := errors.New("index: not a directory")
	if err := retryWhileLocked(context.Background(), time.Second, func() error { calls++; return other }); !errors.Is(err, other) || calls != 1 {
		t.Errorf("other errors must not be retried: %v after %d calls", err, calls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	_ = retryWhileLocked(ctx, 10*time.Second, func() error { calls++; return locked })
	if calls != 1 {
		t.Errorf("a cancelled context retried %d times", calls)
	}
}
