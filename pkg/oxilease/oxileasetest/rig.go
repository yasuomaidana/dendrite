// Package oxileasetest builds a lease manager for tests of code that holds the OxiLLaMa server:
// everything is real (leases, the supervisor loop, a child process) except the server binary,
// whose place a sleeping process takes.
package oxileasetest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"Dendrite/pkg/oxilease"
	"Dendrite/pkg/testkit"
)

// Rig is a manager over a sleeping child process, in a home with two installed models, "small"
// and "big".
type Rig struct {
	M *oxilease.Manager
	// Answering says whether "the server" answers; set it to stand for one started by hand.
	Answering atomic.Bool
	// Fits, when set, is the reason no model may start.
	Fits error
}

// New builds the rig and points HOME at its models. The supervisor is stopped when the test ends.
func New(t *testing.T) *Rig {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for file, body := range map[string]string{"small/small.gguf": "12", "big/big.gguf": "123456"} {
		testkit.WriteFile(t, filepath.Join(home, ".oxillama", "models", file), body, 0o644)
	}
	r := &Rig{}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	r.M = oxilease.New(oxilease.Options{
		// The lease lives two seconds: a loaded runner can take over a second to start the child
		// and to schedule the first heartbeat, and a lease that expired meanwhile is never renewed.
		Dir: t.TempDir(), TTL: 2 * time.Second, IdleGrace: 300 * time.Millisecond, Poll: 10 * time.Millisecond, Port: 8123,
		Healthy: r.Answering.Load,
		Fits:    func(oxilease.Request) error { return r.Fits },
		ChildCmd: func(oxilease.Request) (*exec.Cmd, error) {
			r.Answering.Store(true)
			return exec.Command("sh", "-c", "exec sleep 300"), nil
		},
		Spawn: func(req oxilease.Request) (int, error) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = r.M.Supervise(ctx, req)
				r.Answering.Store(false)
			}()
			return os.Getpid(), nil
		},
	})
	// Registered after the temp dirs, so it runs first: the supervisor stops before they are removed.
	t.Cleanup(func() { cancel(); wg.Wait() })
	return r
}

// Holders lists who holds a lease.
func (r *Rig) Holders(t *testing.T) []string {
	t.Helper()
	hs, err := r.M.Holders()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, h := range hs {
		names = append(names, h.Holder)
	}
	return names
}
