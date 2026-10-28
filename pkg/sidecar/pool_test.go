package sidecar

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type sleepResult struct {
	PID   int `json:"pid"`
	Calls int `json:"calls"`
}

func newTestPool(t *testing.T, mutate func(*PoolConfig)) *Pool {
	t.Helper()
	home := t.TempDir()
	install(t, filepath.Join(home, "bin"), "dendrite-testpool")
	cfg := PoolConfig{
		Binary:  "dendrite-testpool",
		Major:   1,
		Locator: Locator{Home: home, LookPath: noPath},
	}
	if mutate != nil {
		mutate(&cfg)
	}
	p := NewPool(cfg)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func pid(t *testing.T, p *Pool) int {
	t.Helper()
	var out struct {
		PID int `json:"pid"`
	}
	if err := p.Call(context.Background(), "pid", nil, &out); err != nil {
		t.Fatal(err)
	}
	return out.PID
}

func TestPoolKeepsOneProcessAcrossCalls(t *testing.T) {
	p := newTestPool(t, nil)
	first := pid(t, p)
	for i := 0; i < 5; i++ {
		if got := pid(t, p); got != first {
			t.Fatalf("call %d ran on pid %d, want the long-lived %d", i, got, first)
		}
	}
	if s := p.Stats(); s.Starts != 1 || s.Served != 6 {
		t.Errorf("stats = %+v, want 1 start and 6 served", s)
	}
}

func TestPoolBoundedQueueRejectsOverflowAndKeepsOrder(t *testing.T) {
	p := newTestPool(t, func(c *PoolConfig) { c.QueueDepth = 2 })
	call := func(ms int) (sleepResult, error) {
		var out sleepResult
		err := p.Call(context.Background(), "sleep", map[string]int{"ms": ms}, &out)
		return out, err
	}

	var wg sync.WaitGroup
	results := make([]sleepResult, 3)
	errs := make([]error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = call(300)
		}(i)
		time.Sleep(60 * time.Millisecond) // one serving, then two waiting
	}

	if _, err := call(1); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("overflow err = %v, want ErrQueueFull", err)
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("queued call %d: %v", i, errs[i])
		}
		if results[i].Calls != i+1 {
			t.Errorf("call %d was served as number %d: not in arrival order", i, results[i].Calls)
		}
	}
	if s := p.Stats(); s.Rejected != 1 || s.Starts != 1 {
		t.Errorf("stats = %+v, want 1 rejection and a single start", s)
	}
}

func TestPoolCoalescesIdenticalSharedCalls(t *testing.T) {
	p := newTestPool(t, nil)
	const n = 6
	var wg sync.WaitGroup
	got := make([]sleepResult, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = p.CallShared(context.Background(), "sleep", map[string]int{"ms": 150}, &got[i])
		}(i)
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if got[i].Calls != 1 {
			t.Errorf("caller %d saw execution %d, want the single shared one", i, got[i].Calls)
		}
	}
	if s := p.Stats(); s.Served != 1 || s.Coalesced != n-1 {
		t.Errorf("stats = %+v, want 1 execution and %d coalesced", s, n-1)
	}
	// Different params are different work.
	var other sleepResult
	if err := p.CallShared(context.Background(), "sleep", map[string]int{"ms": 1}, &other); err != nil || other.Calls != 2 {
		t.Errorf("distinct request = %+v, %v", other, err)
	}
}

func TestPoolRestartsAfterTheProcessDies(t *testing.T) {
	p := newTestPool(t, nil)
	before := pid(t, p)
	if err := p.Call(context.Background(), "die", nil, nil); err == nil {
		t.Fatal("expected an error from a sidecar that exits mid-request")
	}
	after := pid(t, p)
	if after == before {
		t.Error("the dead process was reused")
	}
	if s := p.Stats(); s.Starts < 2 {
		t.Errorf("starts = %d, want a restart", s.Starts)
	}
}

func TestPoolStopsTheProcessWhenIdle(t *testing.T) {
	p := newTestPool(t, func(c *PoolConfig) { c.IdleTimeout = 80 * time.Millisecond })
	first := pid(t, p)
	time.Sleep(300 * time.Millisecond)
	if second := pid(t, p); second == first {
		t.Error("the idle process was not stopped")
	}
	if s := p.Stats(); s.Starts != 2 {
		t.Errorf("starts = %d, want 2", s.Starts)
	}
}

func TestPoolSkipsRequestsCancelledWhileQueued(t *testing.T) {
	p := newTestPool(t, nil)
	go func() { _ = p.Call(context.Background(), "sleep", map[string]int{"ms": 300}, nil) }()
	time.Sleep(60 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := p.Call(ctx, "sleep", map[string]int{"ms": 300}, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the caller's deadline", err)
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Error("the caller waited for the queue instead of giving up")
	}
	var third sleepResult
	if err := p.Call(context.Background(), "sleep", map[string]int{"ms": 1}, &third); err != nil {
		t.Fatal(err)
	}
	if third.Calls != 2 {
		t.Errorf("third call was execution %d: the cancelled request still ran", third.Calls)
	}
}

func TestPoolReportsUnavailableSidecars(t *testing.T) {
	missing := NewPool(PoolConfig{Binary: "dendrite-absent", Major: 1, Locator: Locator{LookPath: noPath}})
	defer missing.Close()
	if err := missing.Call(context.Background(), "pid", nil, nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("missing binary err = %v, want ErrUnavailable", err)
	}

	old := newTestPool(t, func(c *PoolConfig) {
		c.ExtraEnv = func() []string { return []string{"TESTSIDECAR_PROTOCOL=2.0.0"} }
	})
	if err := old.Call(context.Background(), "pid", nil, nil); !errors.Is(err, ErrUnavailable) || !errors.Is(err, ErrUnavailable) {
		t.Errorf("incompatible protocol err = %v, want ErrUnavailable", err)
	}
}

func TestPoolKeepsTheProcessAfterAnRPCError(t *testing.T) {
	p := newTestPool(t, nil)
	first := pid(t, p)
	var rpcErr *RPCError
	if err := p.Call(context.Background(), "no.such.method", nil, nil); !errors.As(err, &rpcErr) {
		t.Fatalf("err = %v, want an RPCError", err)
	}
	if pid(t, p) != first {
		t.Error("a sidecar-level error restarted a healthy process")
	}
}

func TestPoolCallTimeoutKillsTheWedgedProcessWithoutRetrying(t *testing.T) {
	p := newTestPool(t, func(c *PoolConfig) { c.CallTimeout = 100 * time.Millisecond })
	start := time.Now()
	err := p.Call(context.Background(), "sleep", map[string]int{"ms": 2000}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a deadline error", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("took %v: the timed-out call was retried", time.Since(start))
	}
	pid(t, p) // a fresh process serves the next request
	if s := p.Stats(); s.Starts != 2 {
		t.Errorf("starts = %d, want 2", s.Starts)
	}
}

func TestPoolRefusesWorkAfterClose(t *testing.T) {
	p := newTestPool(t, nil)
	pid(t, p)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Call(context.Background(), "pid", nil, nil); !errors.Is(err, ErrPoolClosed) {
		t.Errorf("err = %v, want ErrPoolClosed", err)
	}
	if err := p.CallShared(context.Background(), "pid", nil, nil); !errors.Is(err, ErrPoolClosed) {
		t.Errorf("shared err = %v, want ErrPoolClosed", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close = %v", err)
	}
}
