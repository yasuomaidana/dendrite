package inference

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrQueueFull is returned when the request queue reaches capacity.
	ErrQueueFull = errors.New("local inference queue is full: system saturated")
	// ErrQueueTimeout is returned when waiting in the queue exceeds the timeout threshold.
	ErrQueueTimeout = errors.New("inference request timed out waiting in queue")
	// ErrQueueClosed is returned when submitting to a closed queue.
	ErrQueueClosed = errors.New("local inference queue is closed")
)

// RequestPriority determines scheduling precedence in the local queue.
type RequestPriority int

const (
	PriorityLow RequestPriority = iota
	PriorityNormal
	PriorityHigh
	PriorityUrgent
)

// QueueConfig configures concurrency bounds and queue depth for local inference engines.
type QueueConfig struct {
	MaxConcurrency int           `json:"max_concurrency"`
	MaxQueueSize   int           `json:"max_queue_size"`
	DefaultTimeout time.Duration `json:"default_timeout"`
}

// DefaultQueueConfig provides optimal default parameters for local hardware (Apple Silicon Metal / CUDA).
func DefaultQueueConfig() QueueConfig {
	return QueueConfig{
		MaxConcurrency: 1, // Single-worker by default to prevent Metal memory thrashing
		MaxQueueSize:   64,
		DefaultTimeout: 90 * time.Second,
	}
}

// QueueMetrics provides real-time telemetry on the queue state.
type QueueMetrics struct {
	ActiveWorkers int   `json:"active_workers"`
	PendingQueue  int   `json:"pending_queue"`
	TotalExecuted int64 `json:"total_executed"`
	TotalRejected int64 `json:"total_rejected"`
	TotalTimedOut int64 `json:"total_timed_out"`
}

type queueItem struct {
	id        int64
	priority  RequestPriority
	enqueuedAt time.Time
	index     int
	task      func(ctx context.Context) error
	done      chan error
	ctx       context.Context
}

type priorityQueue []*queueItem

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// Higher priority first
	if pq[i].priority != pq[j].priority {
		return pq[i].priority > pq[j].priority
	}
	// FIFO for same priority
	return pq[i].enqueuedAt.Before(pq[j].enqueuedAt)
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*queueItem)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// LocalLLMQueue regulates access to local inference engines, preventing hardware saturation and OOMs.
type LocalLLMQueue struct {
	cfg           QueueConfig
	mu            sync.Mutex
	pq            priorityQueue
	cond          *sync.Cond
	activeWorkers int
	closed        bool
	nextID        int64

	totalExecuted atomic.Int64
	totalRejected atomic.Int64
	totalTimedOut atomic.Int64
}

// NewLocalLLMQueue initializes a concurrency-bounded priority queue for local LLMs.
func NewLocalLLMQueue(cfg QueueConfig) *LocalLLMQueue {
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 1
	}
	if cfg.MaxQueueSize <= 0 {
		cfg.MaxQueueSize = 64
	}
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = 90 * time.Second
	}

	q := &LocalLLMQueue{
		cfg: cfg,
		pq:  make(priorityQueue, 0),
	}
	q.cond = sync.NewCond(&q.mu)

	// Launch worker pool
	for i := 0; i < cfg.MaxConcurrency; i++ {
		go q.workerLoop()
	}

	return q
}

func (q *LocalLLMQueue) workerLoop() {
	for {
		q.mu.Lock()
		for len(q.pq) == 0 && !q.closed {
			q.cond.Wait()
		}
		if q.closed && len(q.pq) == 0 {
			q.mu.Unlock()
			return
		}

		item := heap.Pop(&q.pq).(*queueItem)
		q.activeWorkers++
		q.mu.Unlock()

		// Execute task if context is still valid
		select {
		case <-item.ctx.Done():
			q.totalTimedOut.Add(1)
			item.done <- item.ctx.Err()
		default:
			err := item.task(item.ctx)
			q.totalExecuted.Add(1)
			item.done <- err
		}

		q.mu.Lock()
		q.activeWorkers--
		q.mu.Unlock()
	}
}

// Submit enqueues an inference function with the given priority and awaits its execution.
func (q *LocalLLMQueue) Submit(ctx context.Context, priority RequestPriority, task func(ctx context.Context) error) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return ErrQueueClosed
	}

	if len(q.pq) >= q.cfg.MaxQueueSize {
		q.totalRejected.Add(1)
		q.mu.Unlock()
		return ErrQueueFull
	}

	q.nextID++
	done := make(chan error, 1)
	item := &queueItem{
		id:         q.nextID,
		priority:   priority,
		enqueuedAt: time.Now(),
		task:       task,
		done:       done,
		ctx:        ctx,
	}

	heap.Push(&q.pq, item)
	q.cond.Signal()
	q.mu.Unlock()

	// Wait for execution or context cancellation
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		q.mu.Lock()
		// Try to remove item from queue if it hasn't been picked up yet
		if item.index >= 0 && item.index < len(q.pq) && q.pq[item.index] == item {
			heap.Remove(&q.pq, item.index)
			q.totalTimedOut.Add(1)
		}
		q.mu.Unlock()
		return ctx.Err()
	}
}

// Metrics returns snapshot telemetry of the queue.
func (q *LocalLLMQueue) Metrics() QueueMetrics {
	q.mu.Lock()
	defer q.mu.Unlock()
	return QueueMetrics{
		ActiveWorkers: q.activeWorkers,
		PendingQueue:  len(q.pq),
		TotalExecuted: q.totalExecuted.Load(),
		TotalRejected: q.totalRejected.Load(),
		TotalTimedOut: q.totalTimedOut.Load(),
	}
}

// Close gracefully stops the worker pool.
func (q *LocalLLMQueue) Close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

// QueuedInferenceEngine decorates an InferenceEngine with rate limiting and queue management.
type QueuedInferenceEngine struct {
	engine   InferenceEngine
	queue    *LocalLLMQueue
	priority RequestPriority
}

// NewQueuedInferenceEngine wraps an InferenceEngine with the LocalLLMQueue.
func NewQueuedInferenceEngine(engine InferenceEngine, queue *LocalLLMQueue, priority RequestPriority) *QueuedInferenceEngine {
	if queue == nil {
		queue = NewLocalLLMQueue(DefaultQueueConfig())
	}
	return &QueuedInferenceEngine{
		engine:   engine,
		queue:    queue,
		priority: priority,
	}
}

func (q *QueuedInferenceEngine) ID() string           { return q.engine.ID() }
func (q *QueuedInferenceEngine) Type() EngineType     { return q.engine.Type() }
func (q *QueuedInferenceEngine) Config() EngineConfig { return q.engine.Config() }
func (q *QueuedInferenceEngine) IsSelfHosted() bool   { return q.engine.IsSelfHosted() }

func (q *QueuedInferenceEngine) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	var resp *CompletionResponse
	var err error

	execErr := q.queue.Submit(ctx, q.priority, func(c context.Context) error {
		resp, err = q.engine.Generate(c, req)
		return err
	})

	if execErr != nil {
		return nil, execErr
	}
	return resp, nil
}

func (q *QueuedInferenceEngine) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	var streamCh <-chan StreamChunk
	var err error

	execErr := q.queue.Submit(ctx, q.priority, func(c context.Context) error {
		streamCh, err = q.engine.Stream(c, req)
		return err
	})

	if execErr != nil {
		return nil, execErr
	}
	return streamCh, nil
}

func (q *QueuedInferenceEngine) Health(ctx context.Context) EngineHealth {
	return q.engine.Health(ctx)
}

func (q *QueuedInferenceEngine) SupportedModels(ctx context.Context) ([]string, error) {
	return q.engine.SupportedModels(ctx)
}

// QueueMetrics returns telemetry from the underlying queue.
func (q *QueuedInferenceEngine) QueueMetrics() QueueMetrics {
	return q.queue.Metrics()
}
