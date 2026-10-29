// Package oxilease starts the OxiLLaMa server when a session needs it and stops it when the last
// session is done. Sessions hold leases; one supervisor process owns the server and stops it once no
// live lease has existed for the idle grace. Everything that decides is serialized by one file lock.
//
// Rules the tests pin down:
//   - A lease is live while its process is alive AND its heartbeat is younger than the TTL, so a
//     crashed session releases its hold on its own, and a reused pid can only hold it for one TTL.
//   - Releasing a lease never stops the server; the supervisor does, and only when no other live
//     lease exists, re-checked under the lock.
//   - A server Dendrite did not start (no owner record) is never stopped here.
//   - A model that does not fit the memory budget is never started.
package oxilease

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"Dendrite/pkg/projectid"
)

const (
	DefaultTTL       = 10 * time.Minute
	DefaultIdleGrace = 2 * time.Minute
	DefaultPoll      = 5 * time.Second
	DefaultPort      = 8080
	stopWait         = 10 * time.Second
)

// ErrNotFit means the model would starve the machine, so the server was not started.
var ErrNotFit = errors.New("oxillama model does not fit the memory budget")

// Request names the server a session needs.
type Request struct {
	Holder    string // who asks, for status output
	ModelPath string // GGUF file
	ModelID   string // id the server serves it under
}

// Options configures a Manager; every field has a working default.
type Options struct {
	Dir       string        // lease directory, default $DENDRITE_HOME/run/oxillama
	TTL       time.Duration // how long a heartbeat keeps a lease live
	IdleGrace time.Duration // how long the server outlives its last lease
	Poll      time.Duration // supervisor check interval
	Port      int
	Now       func() time.Time
	Healthy   func() bool                                  // is a server answering?
	Alive     func(pid int) bool                           // is the process running?
	Fits      func(Request) error                          // nil error = may start
	Spawn     func(Request) (supervisorPID int, err error) // start a detached supervisor
	ChildCmd  func(Request) (*exec.Cmd, error)             // the server process the supervisor runs
}

type lease struct {
	ID          string `json:"id"`
	PID         int    `json:"pid"`
	Holder      string `json:"holder"`
	HeartbeatMS int64  `json:"heartbeat_ms"`
}

type owner struct {
	SupervisorPID int    `json:"supervisor_pid"`
	ChildPID      int    `json:"child_pid,omitempty"`
	Model         string `json:"model"`
	StartedMS     int64  `json:"started_ms"`
}

// Manager coordinates leases and the supervised server.
type Manager struct{ o Options }

// New fills in the defaults.
func New(o Options) *Manager {
	if o.IdleGrace <= 0 {
		o.IdleGrace = envDuration("DENDRITE_OXILLAMA_IDLE_GRACE")
	}
	if o.TTL <= 0 {
		o.TTL = envDuration("DENDRITE_OXILLAMA_LEASE_TTL")
	}
	if o.Dir == "" {
		if home, err := projectid.HomeDir(); err == nil {
			o.Dir = filepath.Join(home, "run", "oxillama")
		} else {
			o.Dir = filepath.Join(os.TempDir(), "dendrite-oxillama")
		}
	}
	if o.TTL <= 0 {
		o.TTL = DefaultTTL
	}
	if o.IdleGrace <= 0 {
		o.IdleGrace = DefaultIdleGrace
	}
	if o.Poll <= 0 {
		o.Poll = DefaultPoll
	}
	if o.Port == 0 {
		o.Port = DefaultPort
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Healthy == nil {
		port := o.Port
		o.Healthy = func() bool { return healthy(port) }
	}
	if o.Alive == nil {
		o.Alive = processAlive
	}
	if o.Fits == nil {
		o.Fits = func(Request) error { return nil }
	}
	return &Manager{o: o}
}

func envDuration(name string) time.Duration {
	d, err := time.ParseDuration(os.Getenv(name))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

func healthy(port int) bool {
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func (m *Manager) leasesDir() string { return filepath.Join(m.o.Dir, "leases") }
func (m *Manager) ownerPath() string { return filepath.Join(m.o.Dir, "owner.json") }

func (m *Manager) lock() (func(), error) {
	if err := os.MkdirAll(m.leasesDir(), 0o755); err != nil {
		return nil, err
	}
	return flock(filepath.Join(m.o.Dir, "lock"))
}

// liveLeases returns the live leases and deletes the dead ones. The caller holds the lock.
func (m *Manager) liveLeases() []lease {
	entries, _ := os.ReadDir(m.leasesDir())
	now := m.o.Now().UnixMilli()
	var live []lease
	for _, e := range entries {
		path := filepath.Join(m.leasesDir(), e.Name())
		var l lease
		data, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(data, &l) != nil ||
			!m.o.Alive(l.PID) || now-l.HeartbeatMS > m.o.TTL.Milliseconds() {
			_ = os.Remove(path)
			continue
		}
		live = append(live, l)
	}
	return live
}

func (m *Manager) readOwner() *owner {
	data, err := os.ReadFile(m.ownerPath())
	if err != nil {
		return nil
	}
	var o owner
	if json.Unmarshal(data, &o) != nil {
		return nil
	}
	return &o
}

func (m *Manager) writeOwner(o owner) error {
	data, _ := json.Marshal(o)
	tmp := m.ownerPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.ownerPath())
}

// Lease is one session's hold on the server.
type Lease struct {
	m  *Manager
	id string
}

func (m *Manager) writeLease(l lease) error {
	data, _ := json.Marshal(l)
	return os.WriteFile(filepath.Join(m.leasesDir(), l.ID+".json"), data, 0o644)
}

// Acquire takes a lease for the calling process and makes sure a server is running or starting.
// It returns once the server answers. The server may already be one the user started by hand; it
// is then left alone when the lease ends.
func (m *Manager) Acquire(ctx context.Context, req Request) (*Lease, error) {
	unlock, err := m.lock()
	if err != nil {
		return nil, err
	}
	m.liveLeases() // prune the dead before counting
	id := newID()
	l := lease{ID: id, PID: os.Getpid(), Holder: req.Holder, HeartbeatMS: m.o.Now().UnixMilli()}
	if err := m.writeLease(l); err != nil {
		unlock()
		return nil, err
	}
	lease := &Lease{m: m, id: id}

	if !m.o.Healthy() {
		own := m.readOwner()
		if own == nil || !m.o.Alive(own.SupervisorPID) { // nobody is starting one
			if err := m.o.Fits(req); err != nil {
				_ = os.Remove(filepath.Join(m.leasesDir(), id+".json"))
				unlock()
				return nil, fmt.Errorf("%w: %v", ErrNotFit, err)
			}
			_ = os.Remove(m.ownerPath())
			pid, err := m.o.Spawn(req)
			if err != nil {
				_ = os.Remove(filepath.Join(m.leasesDir(), id+".json"))
				unlock()
				return nil, fmt.Errorf("start oxillama supervisor: %w", err)
			}
			// Recorded under the lock so a second Acquire waits instead of starting another.
			_ = m.writeOwner(owner{SupervisorPID: pid, Model: req.ModelID, StartedMS: m.o.Now().UnixMilli()})
		}
	}
	unlock()

	if err := m.waitHealthy(ctx); err != nil {
		_ = lease.Release()
		return nil, err
	}
	return lease, nil
}

func (m *Manager) waitHealthy(ctx context.Context) error {
	deadline := time.Now().Add(120 * time.Second)
	for !m.o.Healthy() {
		if time.Now().After(deadline) {
			return errors.New("oxillama did not become healthy in 120s")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return nil
}

// TTL is how long a heartbeat keeps a lease live.
func (m *Manager) TTL() time.Duration { return m.o.TTL }

// Running reports whether a server answers right now.
func (m *Manager) Running() bool { return m.o.Healthy() }

// Heartbeat keeps a long-running holder's lease live past the TTL.
func (l *Lease) Heartbeat() error {
	unlock, err := l.m.lock()
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(l.m.leasesDir(), l.id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cur lease
	if err := json.Unmarshal(data, &cur); err != nil {
		return err
	}
	cur.HeartbeatMS = l.m.o.Now().UnixMilli()
	return l.m.writeLease(cur)
}

// Release gives the lease up. It is idempotent and never stops the server itself.
func (l *Lease) Release() error {
	if l == nil {
		return nil
	}
	unlock, err := l.m.lock()
	if err != nil {
		return err
	}
	defer unlock()
	err = os.Remove(filepath.Join(l.m.leasesDir(), l.id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Holder describes one live lease for status output.
type Holder struct {
	Holder string
	PID    int
}

// Holders lists the live leases.
func (m *Manager) Holders() ([]Holder, error) {
	unlock, err := m.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	var out []Holder
	for _, l := range m.liveLeases() {
		out = append(out, Holder{l.Holder, l.PID})
	}
	return out, nil
}

// Supervise runs the server as a child and stops it when no live lease has existed for the idle
// grace. It runs in the detached supervisor process and returns when the server is gone.
func (m *Manager) Supervise(ctx context.Context, req Request) error {
	if m.o.ChildCmd == nil {
		return errors.New("oxilease: no ChildCmd")
	}
	unlock, err := m.lock()
	if err != nil {
		return err
	}
	if m.o.Healthy() { // a server appeared meanwhile; it is not ours to own
		_ = os.Remove(m.ownerPath())
		unlock()
		return nil
	}
	cmd, err := m.o.ChildCmd(req)
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		_ = os.Remove(m.ownerPath())
		unlock()
		return fmt.Errorf("start oxillama: %w", err)
	}
	_ = m.writeOwner(owner{SupervisorPID: os.Getpid(), ChildPID: cmd.Process.Pid, Model: req.ModelID, StartedMS: m.o.Now().UnixMilli()})
	unlock()

	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()

	lastLive := m.o.Now()
	tick := time.NewTicker(m.o.Poll)
	defer tick.Stop()
	for {
		select {
		case <-exited: // the server died on its own
			m.clearOwner()
			return nil
		case <-ctx.Done():
			m.stopChild(cmd, exited)
			m.clearOwner()
			return ctx.Err()
		case <-tick.C:
		}
		unlock, err := m.lock()
		if err != nil {
			continue
		}
		if len(m.liveLeases()) > 0 {
			lastLive = m.o.Now()
			unlock()
			continue
		}
		if m.o.Now().Sub(lastLive) < m.o.IdleGrace {
			unlock()
			continue
		}
		// Idle past the grace with the lock held: no Acquire can slip in before the server is gone.
		m.stopChild(cmd, exited)
		_ = os.Remove(m.ownerPath())
		unlock()
		return nil
	}
}

func (m *Manager) clearOwner() {
	if unlock, err := m.lock(); err == nil {
		_ = os.Remove(m.ownerPath())
		unlock()
	}
}

func (m *Manager) stopChild(cmd *exec.Cmd, exited <-chan struct{}) {
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-exited:
	case <-time.After(stopWait):
		_ = cmd.Process.Kill()
		<-exited
	}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Status is a snapshot for `dendrite spike status`.
type Status struct {
	Running   bool     `json:"running"`
	Managed   bool     `json:"managed"` // Dendrite started it and will stop it when idle
	Model     string   `json:"model,omitempty"`
	ChildPID  int      `json:"child_pid,omitempty"`
	Holders   []Holder `json:"holders"`
	IdleGrace string   `json:"idle_grace"`
	LeaseTTL  string   `json:"lease_ttl"`
}

// Status reports the server and who holds it.
func (m *Manager) Status() (Status, error) {
	unlock, err := m.lock()
	if err != nil {
		return Status{}, err
	}
	defer unlock()
	st := Status{Running: m.o.Healthy(), IdleGrace: m.o.IdleGrace.String(), LeaseTTL: m.o.TTL.String(), Holders: []Holder{}}
	if own := m.readOwner(); own != nil && m.o.Alive(own.SupervisorPID) {
		st.Managed, st.Model, st.ChildPID = true, own.Model, own.ChildPID
	}
	for _, l := range m.liveLeases() {
		st.Holders = append(st.Holders, Holder{l.Holder, l.PID})
	}
	return st, nil
}
