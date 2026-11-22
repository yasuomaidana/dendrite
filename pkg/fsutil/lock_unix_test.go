//go:build !windows

package fsutil

import (
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Lock serializes the code between it and its release, also for calls within one process: no
// two holders are ever inside at once. (The counters are atomic because the race detector cannot
// see a file lock.)
func TestLockSerializes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	var inside, entered atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := Lock(path)
			if err != nil {
				t.Error(err)
				return
			}
			defer release()
			if n := inside.Add(1); n != 1 {
				t.Errorf("%d holders inside the lock", n)
			}
			entered.Add(1)
			time.Sleep(time.Millisecond)
			inside.Add(-1)
		}()
	}
	wg.Wait()
	if entered.Load() != 16 {
		t.Fatalf("%d holders entered, want 16", entered.Load())
	}
}

func TestLockNeedsItsDirectory(t *testing.T) {
	if _, err := Lock(filepath.Join(t.TempDir(), "absent", "lock")); err == nil {
		t.Fatal("a lock in a missing directory must fail")
	}
}

// A lock the kernel refuses is an error, and the file opened for it is not left open.
func TestLockReportsARefusedLock(t *testing.T) {
	refused := errors.New("no locks available")
	flockFile = func(int, int) error { return refused }
	t.Cleanup(func() { flockFile = syscallFlock })
	if _, err := Lock(filepath.Join(t.TempDir(), "lock")); !errors.Is(err, refused) {
		t.Fatalf("Lock = %v, want the refusal", err)
	}
}

var syscallFlock = flockFile
