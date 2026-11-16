//go:build !windows

package fsutil

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

// Lock serializes the code between it and its release, also for calls within one process.
func TestLockSerializes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	counter, inside := 0, 0
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
			inside++
			if inside != 1 {
				t.Errorf("%d holders inside the lock", inside)
			}
			counter++
			inside--
		}()
	}
	wg.Wait()
	if counter != 16 {
		t.Fatalf("counter = %d, want 16", counter)
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
