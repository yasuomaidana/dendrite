//go:build !windows

package fsutil

import (
	"os"
	"syscall"
)

// Lock takes an exclusive lock on path (created when missing) and returns its release. It blocks
// while another process or another call in this one holds the lock, so a read-modify-write of a
// file shared between sessions is done between Lock and the release.
func Lock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := flockFile(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = flockFile(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// flockFile is syscall.Flock; tests replace it to reach the case where the kernel refuses the lock.
var flockFile = syscall.Flock
