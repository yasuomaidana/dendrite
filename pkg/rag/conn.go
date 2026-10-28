package rag

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"Dendrite/pkg/sidecar"
)

// conn is a lease on the shared dendrite-rag sidecar. It looks like the one-shot client the
// callers used to start and stop for every call, but Call goes through a long-lived pool: one
// process, a bounded queue, no embedder load per call. Close is a no-op; the pool lives for the
// process (or until idle).
type conn struct {
	pool *sidecar.Pool
	// Path is the sidecar executable and Health its handshake answer.
	Path   string
	Health sidecar.Health
}

func (c *conn) Call(ctx context.Context, method string, params, result interface{}) error {
	return c.pool.Call(ctx, method, params, result)
}

func (c *conn) Close() error { return nil }

type poolEntry struct {
	pool   *sidecar.Pool
	health sidecar.Health
}

var (
	poolsMu sync.Mutex
	pools   = map[string]*poolEntry{}
)

// connect returns a lease on the pooled sidecar at bin, starting it on first use. Pools are
// shared process-wide and keyed by binary and environment.
func (r *Retriever) connect(ctx context.Context, bin string) (*conn, error) {
	key := bin + "\x00" + envFingerprint(r.Env)
	poolsMu.Lock()
	e := pools[key]
	if e == nil {
		env := append([]string(nil), r.Env...)
		e = &poolEntry{pool: sidecar.NewPool(sidecar.PoolConfig{
			Binary:   SidecarBinary,
			Major:    SidecarMajor,
			Path:     bin,
			ExtraEnv: func() []string { return env },
		})}
		pools[key] = e
	}
	poolsMu.Unlock()

	if e.health.Protocol == "" {
		var h sidecar.Health
		if err := e.pool.Call(ctx, "health", nil, &h); err != nil {
			return nil, fmt.Errorf("health: %w", err)
		}
		poolsMu.Lock()
		e.health = h
		poolsMu.Unlock()
	}
	poolsMu.Lock()
	h := e.health
	poolsMu.Unlock()
	return &conn{pool: e.pool, Path: bin, Health: h}, nil
}

// CloseSidecars stops every pooled sidecar. Long-lived hosts call it on shutdown; short-lived
// processes need not, because the sidecar exits when its parent's pipe closes.
func CloseSidecars() {
	poolsMu.Lock()
	all := pools
	pools = map[string]*poolEntry{}
	poolsMu.Unlock()
	for _, e := range all {
		_ = e.pool.Close()
	}
}

// envFingerprint identifies the environment a sidecar would inherit: the process environment plus
// the extra lines. A sidecar reads its settings from the environment it was started with, so a
// changed environment must get a fresh process rather than a pooled one with stale settings.
func envFingerprint(extra []string) string {
	all := append(os.Environ(), extra...)
	sort.Strings(all)
	sum := sha256.Sum256([]byte(strings.Join(all, "\x00")))
	return fmt.Sprintf("%x", sum[:8])
}
