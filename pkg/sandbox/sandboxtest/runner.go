// Package sandboxtest holds the command runner the sandbox and agent tests share: it records every command
// it is asked to run and answers with a chosen output or error.
package sandboxtest

import (
	"context"
	"sync"
)

// RecordingRunner records each command; a "run" command blocks on BlockCh when it is set.
type RecordingRunner struct {
	mu      sync.Mutex
	calls   [][]string
	Output  []byte
	Err     error
	BlockCh chan struct{}
}

// Run records the call, then blocks on BlockCh for a "run" command and returns Output and Err.
func (r *RecordingRunner) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, append([]string{command}, args...))
	r.mu.Unlock()

	if len(args) > 0 && args[0] == "run" && r.BlockCh != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-r.BlockCh:
		}
	}
	return r.Output, r.Err
}

// HasCall reports whether any recorded call (command first) satisfies predicate.
func (r *RecordingRunner) HasCall(predicate func(call []string) bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, call := range r.calls {
		if predicate(call) {
			return true
		}
	}
	return false
}
