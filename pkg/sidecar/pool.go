package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ErrQueueFull is returned when the pool's request queue is at capacity. The sidecar is never
// handed more work than it can take: callers fail fast and use their fallback instead of piling up.
var ErrQueueFull = errors.New("sidecar: request queue is full")

// ErrPoolClosed is returned for requests made to, or still queued in, a closed pool.
var ErrPoolClosed = errors.New("sidecar: pool is closed")

// ErrUnavailable wraps every failure to obtain a running sidecar (binary missing, incompatible
// protocol, failed handshake). Callers treat it as "use the in-process implementation".
var ErrUnavailable = errors.New("sidecar: unavailable")

// PoolConfig describes the sidecar a Pool keeps alive.
type PoolConfig struct {
	// Binary is the sidecar name, e.g. "dendrite-plane".
	Binary string
	// Major is the protocol major version the caller speaks.
	Major int
	// Path, when set, is the executable to run and Locator is not consulted.
	Path string
	// Locator finds the executable; the zero Locator searches PATH only.
	Locator Locator
	// ExtraEnv adds KEY=VALUE lines to the sidecar's environment at each (re)start.
	ExtraEnv func() []string
	// QueueDepth bounds waiting requests (default 16). One more is being served.
	QueueDepth int
	// IdleTimeout stops the process after this long without requests (default 5 minutes);
	// the next request starts it again.
	IdleTimeout time.Duration
	// CallTimeout bounds one request, including its queue-independent execution (default 2 minutes).
	CallTimeout time.Duration
}

// PoolStats are monotonic counters, useful in tests and telemetry.
type PoolStats struct {
	Starts    int64 // sidecar processes started
	Served    int64 // requests executed
	Rejected  int64 // requests refused with ErrQueueFull
	Coalesced int64 // requests that shared another request's execution
}

// Pool keeps one long-lived sidecar process and serves requests to it through a bounded queue,
// one at a time, in arrival order. The process starts on first use, is restarted after it dies,
// and stops after IdleTimeout without requests.
type Pool struct {
	cfg  PoolConfig
	jobs chan *job
	quit chan struct{}
	done chan struct{}

	mu      sync.Mutex
	closed  bool
	flights map[string]*flight

	client *Client // owned by the worker goroutine

	starts, served, rejected, coalesced atomic.Int64
}

type job struct {
	ctx    context.Context
	method string
	params interface{}
	done   chan jobResult
}

type jobResult struct {
	raw json.RawMessage
	err error
}

// flight is one execution shared by every identical concurrent request.
type flight struct {
	done chan struct{}
	raw  json.RawMessage
	err  error
}

// NewPool starts the pool's worker. No process is launched until the first request.
func NewPool(cfg PoolConfig) *Pool {
	if cfg.QueueDepth <= 0 {
		cfg.QueueDepth = 16
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}
	if cfg.CallTimeout <= 0 {
		cfg.CallTimeout = 2 * time.Minute
	}
	p := &Pool{
		cfg:     cfg,
		jobs:    make(chan *job, cfg.QueueDepth),
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
		flights: map[string]*flight{},
	}
	go p.run()
	return p
}

// Stats returns the pool's counters.
func (p *Pool) Stats() PoolStats {
	return PoolStats{Starts: p.starts.Load(), Served: p.served.Load(), Rejected: p.rejected.Load(), Coalesced: p.coalesced.Load()}
}

// Call queues one request and decodes its result into result (which may be nil). It returns
// ErrQueueFull at once when the queue is at capacity and ErrUnavailable when no sidecar can run.
func (p *Pool) Call(ctx context.Context, method string, params, result interface{}) error {
	raw, err := p.submit(ctx, method, params)
	return decodeResult(raw, err, result)
}

// CallShared is Call for read-only requests: concurrent identical requests (same method and
// params) run once and every caller receives the result.
func (p *Pool) CallShared(ctx context.Context, method string, params, result interface{}) error {
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	key := method + "\x00" + string(encoded)

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrPoolClosed
	}
	if f, ok := p.flights[key]; ok {
		p.mu.Unlock()
		p.coalesced.Add(1)
		select {
		case <-f.done:
			return decodeResult(f.raw, f.err, result)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f := &flight{done: make(chan struct{})}
	p.flights[key] = f
	p.mu.Unlock()

	// The leader's cancellation must not fail the followers that joined its flight.
	raw, err := p.submit(context.WithoutCancel(ctx), method, params)
	p.mu.Lock()
	delete(p.flights, key)
	p.mu.Unlock()
	f.raw, f.err = raw, err
	close(f.done)

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return decodeResult(raw, err, result)
}

func decodeResult(raw json.RawMessage, err error, result interface{}) error {
	if err != nil {
		return err
	}
	if result == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, result)
}

func (p *Pool) submit(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return nil, ErrPoolClosed
	}
	j := &job{ctx: ctx, method: method, params: params, done: make(chan jobResult, 1)}
	select {
	case p.jobs <- j:
	default:
		p.rejected.Add(1)
		return nil, ErrQueueFull
	}
	select {
	case r := <-j.done:
		return r.raw, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close stops the worker and the sidecar process. Queued requests fail with ErrPoolClosed.
func (p *Pool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()
	close(p.quit)
	<-p.done
	return nil
}

func (p *Pool) run() {
	defer close(p.done)
	idle := time.NewTimer(p.cfg.IdleTimeout)
	defer idle.Stop()
	for {
		select {
		case <-p.quit:
			p.stopClient()
			p.drain()
			return
		case <-idle.C:
			p.stopClient()
			idle.Reset(p.cfg.IdleTimeout)
		case j := <-p.jobs:
			p.execute(j)
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(p.cfg.IdleTimeout)
		}
	}
}

// drain fails the requests still queued when the pool closes.
func (p *Pool) drain() {
	for {
		select {
		case j := <-p.jobs:
			j.done <- jobResult{err: ErrPoolClosed}
		default:
			return
		}
	}
}

func (p *Pool) stopClient() {
	if p.client != nil {
		_ = p.client.Close()
		p.client = nil
	}
}

func (p *Pool) execute(j *job) {
	if err := j.ctx.Err(); err != nil {
		j.done <- jobResult{err: err}
		return
	}
	// A transport failure (the process died) is retried once on a fresh process; a sidecar
	// answer, even an error, is final.
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		c, err := p.ensure(j.ctx)
		if err != nil {
			j.done <- jobResult{err: fmt.Errorf("%w: %v", ErrUnavailable, err)}
			return
		}
		ctx, cancel := context.WithTimeout(j.ctx, p.cfg.CallTimeout)
		var raw json.RawMessage
		err = c.Call(ctx, j.method, j.params, &raw)
		cancel()
		p.served.Add(1)
		if err == nil {
			j.done <- jobResult{raw: raw}
			return
		}
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) {
			j.done <- jobResult{err: err}
			return
		}
		// The process may be wedged or gone; never reuse it.
		p.stopClient()
		lastErr = err
		// A timeout or cancellation is the caller's budget running out: retrying would only
		// double the wait. Only a process that died gets a second chance.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			break
		}
	}
	j.done <- jobResult{err: lastErr}
}

func (p *Pool) ensure(ctx context.Context) (*Client, error) {
	if p.client != nil {
		return p.client, nil
	}
	path := p.cfg.Path
	if path == "" {
		var err error
		if path, err = p.cfg.Locator.Locate(p.cfg.Binary); err != nil {
			return nil, err
		}
	}
	var env []string
	if p.cfg.ExtraEnv != nil {
		env = p.cfg.ExtraEnv()
	}
	c, err := StartWith(ctx, path, p.cfg.Major, env)
	if err != nil {
		return nil, err
	}
	p.starts.Add(1)
	p.client = c
	return c, nil
}
