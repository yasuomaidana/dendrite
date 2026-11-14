package oxilease

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// env stands in for the machine: a flag for "a server answers", a counter of supervisors started,
// and a real child process (sleep) so stopping is observable.
type env struct {
	m       *Manager
	healthy atomic.Bool
	spawns  atomic.Int32
	fitsErr error
	wg      sync.WaitGroup
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{}
	ctx, cancel := context.WithCancel(context.Background())
	e.m = New(Options{
		Dir:       t.TempDir(),
		TTL:       time.Minute,
		IdleGrace: 200 * time.Millisecond,
		Poll:      20 * time.Millisecond,
		Healthy:   e.healthy.Load,
		Fits:      func(Request) error { return e.fitsErr },
		ChildCmd: func(Request) (*exec.Cmd, error) {
			e.healthy.Store(true)
			return exec.Command("sh", "-c", "exec sleep 300"), nil
		},
	})
	e.m.o.Spawn = func(req Request) (int, error) {
		e.spawns.Add(1)
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			_ = e.m.Supervise(ctx, req)
			e.healthy.Store(false)
		}()
		return os.Getpid(), nil
	}
	// Registered after the temp dir, so it runs first: supervisors stop before the dir is removed.
	t.Cleanup(func() { cancel(); e.wg.Wait() })
	return e
}

var req = Request{Holder: "test", ModelID: "m", ModelPath: "/m.gguf"}

func (e *env) childPID(t *testing.T) int {
	t.Helper()
	pid := 0
	waitFor(t, "the supervisor to record its child", func() bool {
		if own := e.m.readOwner(); own != nil {
			pid = own.ChildPID
		}
		return pid != 0
	})
	return pid
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestFirstAcquireStartsTheServerAndALaterOneReusesIt(t *testing.T) {
	e := newEnv(t)
	a, err := e.m.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.m.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.spawns.Load(); got != 1 {
		t.Errorf("supervisors started = %d, want 1", got)
	}
	_ = a.Release()
	_ = b.Release()
}

func TestServerStaysWhileAnotherSessionHoldsALeaseAndStopsAfterTheLastOne(t *testing.T) {
	e := newEnv(t)
	a, _ := e.m.Acquire(context.Background(), req)
	b, _ := e.m.Acquire(context.Background(), req)
	child := e.childPID(t)

	if err := a.Release(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * e.m.o.IdleGrace) // far past the grace: B still holds it
	if !processAlive(child) || !e.healthy.Load() {
		t.Fatal("the server was stopped while another session was still using it")
	}

	if err := b.Release(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the idle server to stop", func() bool { return !processAlive(child) })
	waitFor(t, "owner record removal", func() bool { return e.m.readOwner() == nil })
	waitFor(t, "the health probe to fail after the child stopped", func() bool { return !e.healthy.Load() })
}

func TestReleaseIsIdempotent(t *testing.T) {
	e := newEnv(t)
	a, _ := e.m.Acquire(context.Background(), req)
	if err := a.Release(); err != nil {
		t.Fatal(err)
	}
	if err := a.Release(); err != nil {
		t.Errorf("second release: %v", err)
	}
	var nilLease *Lease
	if err := nilLease.Release(); err != nil {
		t.Errorf("nil release: %v", err)
	}
}

func TestACrashedSessionsLeaseIsPrunedSoItCannotKeepTheServerAlive(t *testing.T) {
	e := newEnv(t)
	dead := exec.Command("true")
	_ = dead.Run() // a process that has exited: its pid is no longer alive
	stale := lease{ID: "crashed", PID: dead.Process.Pid, Holder: "gone", HeartbeatMS: time.Now().UnixMilli()}
	_ = os.MkdirAll(e.m.leasesDir(), 0o755)
	if err := e.m.writeLease(stale); err != nil {
		t.Fatal(err)
	}

	a, _ := e.m.Acquire(context.Background(), req)
	holders, _ := e.m.Holders()
	if len(holders) != 1 || holders[0].PID != os.Getpid() {
		t.Fatalf("holders = %+v, want only this process", holders)
	}
	child := e.childPID(t)
	_ = a.Release()
	waitFor(t, "stop once only a dead holder remained", func() bool { return !processAlive(child) })
}

func TestALeaseWithoutAHeartbeatExpiresEvenWhenItsPidIsAlive(t *testing.T) {
	e := newEnv(t)
	e.healthy.Store(true)              // an existing server, so the test is only about lease liveness
	e.m.o.TTL = 400 * time.Millisecond // generous: a loaded runner can stall a sleeping test for 100 ms
	_ = os.MkdirAll(e.m.leasesDir(), 0o755)
	old := lease{ID: "old", PID: os.Getpid(), Holder: "silent", HeartbeatMS: time.Now().Add(-time.Hour).UnixMilli()}
	if err := e.m.writeLease(old); err != nil {
		t.Fatal(err)
	}
	if h, _ := e.m.Holders(); len(h) != 0 {
		t.Errorf("a silent lease stayed live: %+v", h)
	}

	a, err := e.m.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := a.Heartbeat(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // 400 ms since Acquire, but only 200 ms since the heartbeat
	if h, _ := e.m.Holders(); len(h) != 1 {
		t.Errorf("a heartbeating lease expired: %+v", h)
	}
	time.Sleep(600 * time.Millisecond) // no more heartbeats
	if h, _ := e.m.Holders(); len(h) != 0 {
		t.Errorf("a lease outlived its TTL: %+v", h)
	}
}

func TestAServerStartedByHandIsNeverStopped(t *testing.T) {
	e := newEnv(t)
	e.healthy.Store(true) // someone else's server, no owner record
	a, err := e.m.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	_ = a.Release()
	time.Sleep(6 * e.m.o.IdleGrace)
	if e.spawns.Load() != 0 {
		t.Error("a supervisor was started for a server that already existed")
	}
	if !e.healthy.Load() {
		t.Error("someone else's server was stopped")
	}
}

func TestAnAcquireAfterShutdownStartsAFreshServer(t *testing.T) {
	e := newEnv(t)
	a, _ := e.m.Acquire(context.Background(), req)
	first := e.childPID(t)
	_ = a.Release()
	waitFor(t, "first server to stop", func() bool { return !processAlive(first) && e.m.readOwner() == nil })

	b, err := e.m.Acquire(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	if got := e.spawns.Load(); got != 2 {
		t.Errorf("supervisors started = %d, want 2", got)
	}
	if second := e.childPID(t); second == first || !processAlive(second) {
		t.Errorf("second server pid %d (first %d) not running", second, first)
	}
}

func TestAModelThatDoesNotFitIsNeverStarted(t *testing.T) {
	e := newEnv(t)
	e.fitsErr = errors.New("needs 18 GB")
	_, err := e.m.Acquire(context.Background(), req)
	if !errors.Is(err, ErrNotFit) {
		t.Fatalf("err = %v, want ErrNotFit", err)
	}
	if e.spawns.Load() != 0 {
		t.Error("a supervisor was started anyway")
	}
	if h, _ := e.m.Holders(); len(h) != 0 {
		t.Errorf("a lease was left behind: %+v", h)
	}
}

func TestConcurrentAcquiresStartExactlyOneSupervisor(t *testing.T) {
	e := newEnv(t)
	const n = 8
	done := make(chan *Lease, n)
	for i := 0; i < n; i++ {
		go func() {
			l, err := e.m.Acquire(context.Background(), req)
			if err != nil {
				t.Error(err)
			}
			done <- l
		}()
	}
	var leases []*Lease
	for i := 0; i < n; i++ {
		leases = append(leases, <-done)
	}
	if got := e.spawns.Load(); got != 1 {
		t.Errorf("supervisors started = %d, want 1", got)
	}
	for _, l := range leases {
		_ = l.Release()
	}
}

func TestOwnerRecordPointsAtTheRunningChild(t *testing.T) {
	e := newEnv(t)
	a, _ := e.m.Acquire(context.Background(), req)
	defer a.Release()
	child := e.childPID(t)
	data, err := os.ReadFile(filepath.Join(e.m.o.Dir, "owner.json"))
	if err != nil {
		t.Fatal(err)
	}
	var o owner
	if err := json.Unmarshal(data, &o); err != nil || o.ChildPID != child || o.Model != "m" || !processAlive(child) {
		t.Errorf("owner = %+v, %v", o, err)
	}
}
