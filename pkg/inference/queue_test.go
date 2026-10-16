package inference

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalLLMQueue_ConcurrencyLimit(t *testing.T) {
	cfg := QueueConfig{
		MaxConcurrency: 2,
		MaxQueueSize:   10,
		DefaultTimeout: 5 * time.Second,
	}
	q := NewLocalLLMQueue(cfg)
	defer q.Close()

	var activeCount atomic.Int32
	var maxActive atomic.Int32

	tasks := 8
	var wg sync.WaitGroup
	wg.Add(tasks)

	for i := 0; i < tasks; i++ {
		go func() {
			defer wg.Done()
			err := q.Submit(context.Background(), PriorityNormal, func(ctx context.Context) error {
				curr := activeCount.Add(1)
				for {
					old := maxActive.Load()
					if curr <= old || maxActive.CompareAndSwap(old, curr) {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
				activeCount.Add(-1)
				return nil
			})
			if err != nil {
				t.Errorf("task failed: %v", err)
			}
		}()
	}

	wg.Wait()

	if max := maxActive.Load(); max > 2 {
		t.Fatalf("concurrency violated: observed max %d, expected at most 2", max)
	}

	metrics := q.Metrics()
	if metrics.TotalExecuted != int64(tasks) {
		t.Fatalf("expected %d executed, got %d", tasks, metrics.TotalExecuted)
	}
}

func TestLocalLLMQueue_PriorityPreemption(t *testing.T) {
	cfg := QueueConfig{
		MaxConcurrency: 1, // Single worker so ordering is strictly sequential
		MaxQueueSize:   10,
		DefaultTimeout: 5 * time.Second,
	}
	q := NewLocalLLMQueue(cfg)
	defer q.Close()

	// Block the single worker initially
	blockerRelease := make(chan struct{})
	workerStarted := make(chan struct{})

	go func() {
		_ = q.Submit(context.Background(), PriorityNormal, func(ctx context.Context) error {
			close(workerStarted)
			<-blockerRelease
			return nil
		})
	}()

	<-workerStarted

	// Now enqueue Low priority, then High priority, then Urgent priority
	var executionOrder []string
	var orderMu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		_ = q.Submit(context.Background(), PriorityLow, func(ctx context.Context) error {
			orderMu.Lock()
			executionOrder = append(executionOrder, "LOW")
			orderMu.Unlock()
			return nil
		})
	}()

	time.Sleep(10 * time.Millisecond)

	go func() {
		defer wg.Done()
		_ = q.Submit(context.Background(), PriorityNormal, func(ctx context.Context) error {
			orderMu.Lock()
			executionOrder = append(executionOrder, "NORMAL")
			orderMu.Unlock()
			return nil
		})
	}()

	time.Sleep(10 * time.Millisecond)

	go func() {
		defer wg.Done()
		_ = q.Submit(context.Background(), PriorityUrgent, func(ctx context.Context) error {
			orderMu.Lock()
			executionOrder = append(executionOrder, "URGENT")
			orderMu.Unlock()
			return nil
		})
	}()

	time.Sleep(20 * time.Millisecond)

	// Release the blocker
	close(blockerRelease)
	wg.Wait()

	orderMu.Lock()
	defer orderMu.Unlock()

	if len(executionOrder) != 3 {
		t.Fatalf("expected 3 tasks executed, got %d", len(executionOrder))
	}

	// High priority tasks must jump ahead of low priority tasks
	if executionOrder[0] != "URGENT" {
		t.Errorf("expected first task to be URGENT, got %s", executionOrder[0])
	}
	if executionOrder[1] != "NORMAL" {
		t.Errorf("expected second task to be NORMAL, got %s", executionOrder[1])
	}
	if executionOrder[2] != "LOW" {
		t.Errorf("expected third task to be LOW, got %s", executionOrder[2])
	}
}

func TestLocalLLMQueue_CapacityRejection(t *testing.T) {
	cfg := QueueConfig{
		MaxConcurrency: 1,
		MaxQueueSize:   2,
		DefaultTimeout: 5 * time.Second,
	}
	q := NewLocalLLMQueue(cfg)
	defer q.Close()

	blocker := make(chan struct{})
	started := make(chan struct{})

	// Task 1: occupies the worker
	go func() {
		_ = q.Submit(context.Background(), PriorityNormal, func(ctx context.Context) error {
			close(started)
			<-blocker
			return nil
		})
	}()

	<-started

	// Fill queue to max capacity (2 items)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_ = q.Submit(context.Background(), PriorityNormal, func(ctx context.Context) error {
				return nil
			})
		}()
	}

	time.Sleep(30 * time.Millisecond)

	// Task 4: Queue should now be full and reject immediately
	err := q.Submit(context.Background(), PriorityNormal, func(ctx context.Context) error {
		return nil
	})

	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}

	metrics := q.Metrics()
	if metrics.TotalRejected != 1 {
		t.Errorf("expected 1 rejected, got %d", metrics.TotalRejected)
	}

	close(blocker)
	wg.Wait()
}

func TestLocalLLMQueue_ContextTimeout(t *testing.T) {
	cfg := QueueConfig{
		MaxConcurrency: 1,
		MaxQueueSize:   5,
		DefaultTimeout: 5 * time.Second,
	}
	q := NewLocalLLMQueue(cfg)
	defer q.Close()

	blocker := make(chan struct{})
	started := make(chan struct{})

	// Occupy worker
	go func() {
		_ = q.Submit(context.Background(), PriorityNormal, func(ctx context.Context) error {
			close(started)
			<-blocker
			return nil
		})
	}()

	<-started

	// Submit task with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	err := q.Submit(ctx, PriorityNormal, func(ctx context.Context) error {
		return nil
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}

	close(blocker)
}

// Mock engine for wrapper testing
type mockEngine struct {
	id string
}

func (m *mockEngine) ID() string           { return m.id }
func (m *mockEngine) Type() EngineType     { return EngineOllama }
func (m *mockEngine) Config() EngineConfig { return EngineConfig{ID: m.id} }
func (m *mockEngine) IsSelfHosted() bool   { return true }
func (m *mockEngine) Generate(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	return &CompletionResponse{Text: "generated-" + req.Prompt}, nil
}
func (m *mockEngine) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 1)
	ch <- StreamChunk{Text: "chunk", Done: true}
	close(ch)
	return ch, nil
}
func (m *mockEngine) Health(ctx context.Context) EngineHealth {
	return EngineHealth{EngineID: m.id, Reachable: true}
}
func (m *mockEngine) SupportedModels(ctx context.Context) ([]string, error) {
	return []string{"mock-model"}, nil
}

func TestQueuedInferenceEngine(t *testing.T) {
	raw := &mockEngine{id: "mock-ollama"}
	q := NewLocalLLMQueue(QueueConfig{MaxConcurrency: 2, MaxQueueSize: 10})
	defer q.Close()

	wrapped := NewQueuedInferenceEngine(raw, q, PriorityHigh)

	if wrapped.ID() != "mock-ollama" {
		t.Fatalf("ID mismatch: %s", wrapped.ID())
	}
	if !wrapped.IsSelfHosted() {
		t.Fatal("expected self-hosted to be true")
	}

	// Test Generate through queue
	resp, err := wrapped.Generate(context.Background(), CompletionRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Text != "generated-hello" {
		t.Fatalf("unexpected text: %s", resp.Text)
	}

	// Test Stream through queue
	st, err := wrapped.Stream(context.Background(), CompletionRequest{Prompt: "stream"})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	chunk := <-st
	if chunk.Text != "chunk" {
		t.Fatalf("unexpected chunk text: %s", chunk.Text)
	}

	// Verify health & models pass through
	health := wrapped.Health(context.Background())
	if !health.Reachable {
		t.Fatal("health check failed")
	}

	models, _ := wrapped.SupportedModels(context.Background())
	if len(models) == 0 || models[0] != "mock-model" {
		t.Fatalf("unexpected models: %v", models)
	}

	// Telemetry check
	metrics := wrapped.QueueMetrics()
	if metrics.TotalExecuted != 2 {
		t.Fatalf("expected 2 total executed, got %d", metrics.TotalExecuted)
	}
}

func TestLocalLLMQueue_EdgeCases(t *testing.T) {
	// Test DefaultQueueConfig
	def := DefaultQueueConfig()
	if def.MaxConcurrency != 1 || def.MaxQueueSize != 64 {
		t.Fatalf("unexpected defaults: %+v", def)
	}

	// Test NewLocalLLMQueue default sanitization
	q := NewLocalLLMQueue(QueueConfig{})
	defer q.Close()

	if q.cfg.MaxConcurrency != 1 || q.cfg.MaxQueueSize != 64 {
		t.Fatalf("unexpected sanitized config: %+v", q.cfg)
	}

	// Test Submit on closed queue
	q.Close()
	err := q.Submit(context.Background(), PriorityNormal, func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("expected ErrQueueClosed, got %v", err)
	}

	// Test QueuedEngine methods
	raw := &mockEngine{id: "mock-engine"}
	wrapped := NewQueuedInferenceEngine(raw, nil, PriorityLow)
	if wrapped.Type() != EngineOllama {
		t.Fatalf("expected EngineOllama, got %v", wrapped.Type())
	}
	if wrapped.Config().ID != "mock-engine" {
		t.Fatalf("unexpected config ID: %s", wrapped.Config().ID)
	}
}
