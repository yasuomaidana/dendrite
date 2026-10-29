package agent_test

import (
	"Dendrite/pkg/sandbox/sandboxtest"
	"context"
	"errors"
	"testing"
	"time"

	"Dendrite/pkg/agent"
	"Dendrite/pkg/gateway"
)

func TestNewSandboxedRunner_Defaults(t *testing.T) {
	sbRunner := agent.NewSandboxedRunner(nil, nil)
	if sbRunner == nil {
		t.Fatal("expected non-nil runner")
	}
	if sbRunner.SandboxRunner == nil {
		t.Fatal("expected non-nil default SandboxRunner")
	}
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
		{agent.AgentCopilot, "ghcr.io/github/copilot-cli:latest"},
		{agent.AgentGitHubCopilot, "ghcr.io/github/copilot-cli:latest"},
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

	runner := &sandboxtest.RecordingRunner{Output: []byte("agent completed task"), Err: nil}
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

	runner := &sandboxtest.RecordingRunner{Output: []byte("agy task executed"), Err: nil}
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

func TestSandboxedRunner_BudgetCapFallsBackToAZeroCostModel(t *testing.T) {
	for name, tc := range map[string]struct {
		key, exhausted, fallback string
		cfg                      agent.AgentConfig
	}{
		"configured fallback": {"key-capped", "gemini-2.5-pro", "local-qwen-coder", agent.AgentConfig{Model: "gemini-2.5-pro", FallbackModel: "local-qwen-coder"}},
		// No model and no FallbackModel on the config: the limiter's default model and its fallback apply.
		"limiter default": {"key-auto-fallback", "gemini-3.8-flash-high", "local-qwen3-27b", agent.AgentConfig{}},
	} {
		t.Run(name, func(t *testing.T) {
			limiter := gateway.NewBudgetLimiter()
			limiter.RegisterKey(gateway.VirtualKeyConfig{
				KeyAlias:     tc.key,
				BudgetLimits: []gateway.BudgetLimit{{Duration: gateway.Window24h, MaxBudgetUSD: 1.00}},
				Fallbacks:    map[string][]string{tc.exhausted: {tc.fallback}},
			})
			limiter.RecordSpend(tc.key, tc.exhausted, 5000, 2.00) // $2.00 > $1.00: the budget is gone

			cfg := tc.cfg
			cfg.Type, cfg.LiteLLMBaseURL, cfg.VirtualAPIKey, cfg.WorkspaceDir, cfg.Sandbox = agent.AgentOpenCode, "http://localhost:4000", tc.key, "/tmp/workspace", true
			sbRunner := agent.NewSandboxedRunner(limiter, &sandboxtest.RecordingRunner{Output: []byte("ok")})
			summary, err := sbRunner.Execute(context.Background(), cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if summary.EstimatedCostUSD != 0.0 {
				t.Errorf("expected $0.00 cost for %s, got %.4f", tc.fallback, summary.EstimatedCostUSD)
			}
		})
	}
}

func TestSandboxedRunner_BudgetLimiterError(t *testing.T) {
	limiter := gateway.NewBudgetLimiter()
	// Unregistered key causes CheckBudget to fail
	runner := &sandboxtest.RecordingRunner{Output: []byte("ok"), Err: nil}
	sbRunner := agent.NewSandboxedRunner(limiter, runner)

	cfg := agent.AgentConfig{
		Type:           agent.AgentAider,
		LiteLLMBaseURL: "http://localhost:4000",
		VirtualAPIKey:  "err-unregistered",
		WorkspaceDir:   "/tmp/workspace",
		Sandbox:        true,
	}

	ctx := context.Background()
	_, err := sbRunner.Execute(ctx, cfg)
	if err == nil {
		t.Fatal("expected error when budget limiter check fails")
	}
}

func TestSandboxedRunner_InvalidConfig(t *testing.T) {
	sbRunner := agent.NewSandboxedRunner(nil, nil)
	_, err := sbRunner.Execute(context.Background(), agent.AgentConfig{})
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestSandboxedRunner_ContextCancellationAndCurfew(t *testing.T) {
	blockCh := make(chan struct{})
	runner := &sandboxtest.RecordingRunner{BlockCh: blockCh}
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
	runner := &sandboxtest.RecordingRunner{Output: []byte("host run ok"), Err: nil}
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

func TestSandboxedRunner_ACommandFailureIsAFailedRun(t *testing.T) {
	for name, tc := range map[string]struct {
		err     string
		agent   agent.AgentType
		sandbox bool
	}{
		"on the host":    {"host command failed", agent.AgentOpenCode, false},
		"in the sandbox": {"container process crashed", agent.AgentAider, true},
	} {
		t.Run(name, func(t *testing.T) {
			sbRunner := agent.NewSandboxedRunner(nil, &sandboxtest.RecordingRunner{Err: errors.New(tc.err)})
			summary, err := sbRunner.Execute(context.Background(), agent.AgentConfig{
				Type: tc.agent, LiteLLMBaseURL: "http://localhost:4000", VirtualAPIKey: "key", WorkspaceDir: "/tmp/workspace", Sandbox: tc.sandbox,
			})
			if err == nil {
				t.Fatal("expected an error on a failed command")
			}
			if summary == nil || summary.Status != "failed" {
				t.Errorf("expected status 'failed', got %v", summary)
			}
		})
	}
}
