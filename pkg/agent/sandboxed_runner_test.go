package agent_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/gateway"
)

type mockCmdRunner struct {
	mu      sync.Mutex
	calls   [][]string
	output  []byte
	err     error
	blockCh chan struct{}
}

func (m *mockCmdRunner) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	m.mu.Lock()
	call := append([]string{command}, args...)
	m.calls = append(m.calls, call)
	m.mu.Unlock()

	// Only block if this is the "run" command and blockCh is set
	if len(args) > 0 && args[0] == "run" && m.blockCh != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.blockCh:
		}
	}

	return m.output, m.err
}

func (m *mockCmdRunner) HasCall(predicate func(args []string) bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, call := range m.calls {
		if predicate(call) {
			return true
		}
	}
	return false
}

func TestSelectDefaultSandboxImage(t *testing.T) {
	cases := []struct {
		agentType agent.AgentType
		expected  string
	}{
		{agent.AgentOpenHands, "docker.all-hands.dev/all-hands-ai/runtime:0.14-nikolaik"},
		{agent.AgentAider, "paulgauthier/aider:latest"},
		{agent.AgentOpenCode, "golang:1.24-alpine"},
		{agent.AgentAGY, "google/antigravity:latest"},
		{agent.AgentAntigravity, "google/antigravity:latest"},
		{"unknown", "alpine:latest"},
	}

	for _, c := range cases {
		img := agent.SelectDefaultSandboxImage(c.agentType)
		if img != c.expected {
			t.Errorf("for %s: expected image %s, got %s", c.agentType, c.expected, img)
		}
	}
}

func TestSandboxedRunner_ExecuteSuccess(t *testing.T) {
	limiter := gateway.NewBudgetLimiter()
	limiter.RegisterKey(gateway.VirtualKeyConfig{
		KeyAlias: "key-1",
		BudgetLimits: []gateway.BudgetLimit{
			{Duration: gateway.Window24h, MaxBudgetUSD: 10.00},
		},
	})

	runner := &mockCmdRunner{output: []byte("agent completed task"), err: nil}
	sbRunner := agent.NewSandboxedRunner(limiter, runner)

	cfg := agent.AgentConfig{
		Type:           agent.AgentAider,
		LiteLLMBaseURL: "http://localhost:4000",
		VirtualAPIKey:  "key-1",
		WorkspaceDir:   "/tmp/workspace",
		Model:          "gemini-2.5-pro",
		TicketID:       "DEN-025",
		Sandbox:        true,
	}

	ctx := context.Background()
	summary, err := sbRunner.Execute(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error executing sandboxed runner: %v", err)
	}

	if summary.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", summary.Status)
	}
	if summary.TicketID != "DEN-025" {
		t.Errorf("expected ticket ID DEN-025, got %s", summary.TicketID)
	}

	// Verify RAII cleanup was triggered (docker rm -f)
	cleaned := runner.HasCall(func(args []string) bool {
		return len(args) >= 3 && args[0] == "docker" && args[1] == "rm" && args[2] == "-f"
	})
	if !cleaned {
		t.Errorf("expected docker rm -f cleanup call during session teardown")
	}

	// Verify spend recorded in budget limiter
	spent, _ := limiter.GetWindowSpend("key-1", gateway.Window24h)
	if spent <= 0.0 {
		t.Errorf("expected spend recorded in budget limiter, got %.4f", spent)
	}
}

func TestSandboxedRunner_ExecuteAGYSuccess(t *testing.T) {
	limiter := gateway.NewBudgetLimiter()
	limiter.RegisterKey(gateway.VirtualKeyConfig{
		KeyAlias: "key-agy",
		BudgetLimits: []gateway.BudgetLimit{
			{Duration: gateway.Window24h, MaxBudgetUSD: 20.00},
		},
	})

	runner := &mockCmdRunner{output: []byte("agy task executed"), err: nil}
	sbRunner := agent.NewSandboxedRunner(limiter, runner)

	cfg := agent.AgentConfig{
		Type:            agent.AgentAGY,
		LiteLLMBaseURL:  "http://localhost:4000",
		VirtualAPIKey:   "key-agy",
		WorkspaceDir:    "/tmp/workspace",
		Model:           "gemini-2.5-pro",
		TicketID:        "DEN-028",
		TaskDescription: "Execute autonomous plan",
		Sandbox:         true,
	}

	ctx := context.Background()
	summary, err := sbRunner.Execute(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error executing sandboxed runner with AGY: %v", err)
	}

	if summary.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", summary.Status)
	}
	if summary.TicketID != "DEN-028" {
		t.Errorf("expected ticket ID DEN-028, got %s", summary.TicketID)
	}
	if summary.AgentType != agent.AgentAGY {
		t.Errorf("expected agent type agy, got %s", summary.AgentType)
	}

	cleaned := runner.HasCall(func(args []string) bool {
		return len(args) >= 3 && args[0] == "docker" && args[1] == "rm" && args[2] == "-f"
	})
	if !cleaned {
		t.Errorf("expected docker rm -f cleanup call during session teardown")
	}
}

func TestSandboxedRunner_BudgetCapFallback(t *testing.T) {
	limiter := gateway.NewBudgetLimiter()
	limiter.RegisterKey(gateway.VirtualKeyConfig{
		KeyAlias: "key-capped",
		BudgetLimits: []gateway.BudgetLimit{
			{Duration: gateway.Window24h, MaxBudgetUSD: 1.00},
		},
		Fallbacks: map[string][]string{
			"gemini-2.5-pro": {"local-qwen-coder"},
		},
	})
	// Exhaust budget immediately ($2.00 > $1.00)
	limiter.RecordSpend("key-capped", "gemini-2.5-pro", 5000, 2.00)

	runner := &mockCmdRunner{output: []byte("ok"), err: nil}
	sbRunner := agent.NewSandboxedRunner(limiter, runner)

	cfg := agent.AgentConfig{
		Type:           agent.AgentOpenCode,
		LiteLLMBaseURL: "http://localhost:4000",
		VirtualAPIKey:  "key-capped",
		WorkspaceDir:   "/tmp/workspace",
		Model:          "gemini-2.5-pro",
		FallbackModel:  "local-qwen-coder",
		Sandbox:        true,
	}

	ctx := context.Background()
	summary, err := sbRunner.Execute(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Zero-cost model should be selected due to budget cap
	if summary.EstimatedCostUSD != 0.0 {
		t.Errorf("expected $0.00 cost for local-qwen-coder fallback, got %.4f", summary.EstimatedCostUSD)
	}
}

func TestSandboxedRunner_ContextCancellationAndCurfew(t *testing.T) {
	blockCh := make(chan struct{})
	runner := &mockCmdRunner{blockCh: blockCh}
	sbRunner := agent.NewSandboxedRunner(nil, runner)

	cfg := agent.AgentConfig{
		Type:           agent.AgentOpenHands,
		LiteLLMBaseURL: "http://localhost:4000",
		VirtualAPIKey:  "key-curfew",
		WorkspaceDir:   "/tmp/workspace",
		Sandbox:        true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	summary, err := sbRunner.Execute(ctx, cfg)
	if err == nil {
		t.Errorf("expected error on curfew cancellation, got nil")
	}
	if summary == nil || summary.Status != "curfew_interrupted" {
		t.Errorf("expected status 'curfew_interrupted', got %v", summary)
	}

	// Verify container cleanup was still guaranteed
	cleaned := runner.HasCall(func(args []string) bool {
		return len(args) >= 3 && args[0] == "docker" && args[1] == "rm" && args[2] == "-f"
	})
	if !cleaned {
		t.Errorf("expected docker rm -f cleanup call on curfew cancel")
	}
}

func TestSandboxedRunner_HostExecution(t *testing.T) {
	runner := &mockCmdRunner{output: []byte("host run ok"), err: nil}
	sbRunner := agent.NewSandboxedRunner(nil, runner)

	cfg := agent.AgentConfig{
		Type:           agent.AgentOpenCode,
		LiteLLMBaseURL: "http://localhost:4000",
		VirtualAPIKey:  "key-host",
		WorkspaceDir:   "/tmp/workspace",
		Sandbox:        false, // Direct host execution
	}

	ctx := context.Background()
	summary, err := sbRunner.Execute(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error on host execution: %v", err)
	}
	if summary.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", summary.Status)
	}
}

func TestSandboxedRunner_ExecutionFailure(t *testing.T) {
	runner := &mockCmdRunner{err: errors.New("container process crashed")}
	sbRunner := agent.NewSandboxedRunner(nil, runner)

	cfg := agent.AgentConfig{
		Type:           agent.AgentAider,
		LiteLLMBaseURL: "http://localhost:4000",
		VirtualAPIKey:  "key-err",
		WorkspaceDir:   "/tmp/workspace",
		Sandbox:        true,
	}

	ctx := context.Background()
	summary, err := sbRunner.Execute(ctx, cfg)
	if err == nil {
		t.Errorf("expected error on failed execution")
	}
	if summary == nil || summary.Status != "failed" {
		t.Errorf("expected status 'failed', got %v", summary)
	}
}
