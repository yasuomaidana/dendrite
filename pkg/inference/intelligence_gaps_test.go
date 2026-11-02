package inference

import (
	"context"
	"errors"
	"testing"
	"time"
)

// scriptedEngine is an InferenceEngine whose models and outcomes the test sets.
type scriptedEngine struct {
	id        string
	typ       EngineType
	models    []string
	selfHost  bool
	reachable bool
	failWith  error
	calls     []string
}

func (e *scriptedEngine) ID() string           { return e.id }
func (e *scriptedEngine) Type() EngineType     { return e.typ }
func (e *scriptedEngine) Config() EngineConfig { return EngineConfig{ID: e.id, Models: e.models} }
func (e *scriptedEngine) IsSelfHosted() bool   { return e.selfHost }
func (e *scriptedEngine) Health(context.Context) EngineHealth {
	return EngineHealth{EngineID: e.id, Reachable: e.reachable}
}
func (e *scriptedEngine) SupportedModels(context.Context) ([]string, error) {
	return nil, errors.New("not asked")
}
func (e *scriptedEngine) Stream(context.Context, CompletionRequest) (<-chan StreamChunk, error) {
	return nil, errors.New("not streamed")
}
func (e *scriptedEngine) Generate(_ context.Context, req CompletionRequest) (*CompletionResponse, error) {
	e.calls = append(e.calls, req.Model)
	if e.failWith != nil {
		return nil, e.failWith
	}
	return &CompletionResponse{EngineID: e.id, Model: req.Model, Text: "ok", CreatedAt: time.Now()}, nil
}

func TestModelIntelligenceScoreFallsBackToNamesWithoutAParameterCount(t *testing.T) {
	cases := map[string]int{
		"x27bit":  88, // 27B
		"x32bit":  92,
		"x14bit":  78,
		"x9bit":   65,
		"x8bit":   65,
		"x6.7bit": 60,
		"x7bit":   60,
		"x3bit":   38,
		"x1.5bit": 25,
		"":        10,
	}
	for name, want := range cases {
		if got := ModelIntelligenceScore(name); got != want {
			t.Errorf("ModelIntelligenceScore(%q) = %d, want %d", name, got, want)
		}
	}
	if got := ModelIntelligenceScore("qwen3-coder-405b"); got > 100 {
		t.Errorf("scores stay within 100, got %d", got)
	}
}

func TestEnginesAreOrderedByLivenessGPUScoreThenPreference(t *testing.T) {
	ctx := context.Background()
	live := func(id string, typ EngineType, models ...string) *scriptedEngine {
		return &scriptedEngine{id: id, typ: typ, models: models, selfHost: true, reachable: true}
	}
	engines := []InferenceEngine{
		&scriptedEngine{id: "down", typ: EngineOllama, models: []string{"qwen3:32b"}},
		live("oxi", EngineOxiLLaMa, "qwen3:32b"),
		live("llama", EngineLlamaCpp, "small:7b"),
		live("ollama-same", EngineOllama, "small:7b"),
		live("big", EngineVLLM, "qwen3:32b"),
	}
	got := ReorderEnginesByIntelligence(ctx, engines)
	var ids []string
	for _, e := range got {
		ids = append(ids, e.ID())
	}
	want := []string{"big", "ollama-same", "llama", "oxi", "down"}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order = %v, want %v", ids, want)
		}
	}
}

func TestSmartCascadeCountsOnlyFailuresOfTheSmartestModels(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	smart := &scriptedEngine{id: "smart", typ: EngineOllama, models: []string{"qwen3-coder:32b"}, selfHost: true, failWith: boom}
	small := &scriptedEngine{id: "small", typ: EngineVLLM, models: []string{"tiny:1b"}, selfHost: true, failWith: boom}
	remote := &scriptedEngine{id: "remote", typ: EngineVLLM, models: []string{"qwen3:32b"}}
	cpu := &scriptedEngine{id: "cpu", typ: EngineOxiLLaMa, models: []string{"qwen3:32b"}, selfHost: true, failWith: boom}

	paid := func(_ context.Context, _ CompletionRequest, model string) (*CompletionResponse, error) {
		return &CompletionResponse{Model: model, Text: "paid"}, nil
	}
	req := CompletionRequest{Prompt: "hi"}

	t.Run("small failures are ignored and the smart one triggers the escalation", func(t *testing.T) {
		resp, report, err := ExecuteSmartCascade(ctx, []InferenceEngine{small, smart, remote, cpu}, req, paid, SmartCascadeConfig{MaxSmartFailures: 9})
		if err != nil || resp.Text != "paid" || !report.LeveragedToPaid {
			t.Fatalf("resp = %v, report = %+v, err = %v", resp, report, err)
		}
		if report.MaxSmartThreshold != 3 {
			t.Errorf("the failure limit is capped at 3, got %d", report.MaxSmartThreshold)
		}
		if report.SmartFailuresCounted < 1 {
			t.Errorf("report = %+v", report)
		}
	})
	t.Run("defaults apply to an empty config", func(t *testing.T) {
		_, report, err := ExecuteSmartCascade(ctx, []InferenceEngine{smart}, req, paid, SmartCascadeConfig{})
		if err != nil || report.MaxSmartThreshold != 2 {
			t.Fatalf("report = %+v, err = %v", report, err)
		}
	})
	t.Run("a failing paid model is reported", func(t *testing.T) {
		failing := func(context.Context, CompletionRequest, string) (*CompletionResponse, error) { return nil, boom }
		if _, _, err := ExecuteSmartCascade(ctx, []InferenceEngine{smart}, req, failing, DefaultSmartCascadeConfig()); err == nil {
			t.Fatal("expected the paid failure")
		}
	})
	t.Run("without a paid caller the cascade is exhausted", func(t *testing.T) {
		_, _, err := ExecuteSmartCascade(ctx, []InferenceEngine{small}, req, nil, DefaultSmartCascadeConfig())
		if !errors.Is(err, ErrNoAvailableEngine) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("a working model answers", func(t *testing.T) {
		ok := &scriptedEngine{id: "ok", typ: EngineOllama, models: []string{"qwen3:14b"}, selfHost: true}
		resp, report, err := ExecuteSmartCascade(ctx, []InferenceEngine{ok}, req, nil, DefaultSmartCascadeConfig())
		if err != nil || resp.Text != "ok" || report.SelectedEngine != "ok" {
			t.Fatalf("resp = %v, report = %+v, err = %v", resp, report, err)
		}
	})
	t.Run("a low-scoring field is all treated as smart", func(t *testing.T) {
		weak := &scriptedEngine{id: "weak", typ: EngineOllama, models: []string{"tiny:1b"}, selfHost: true, failWith: boom}
		_, report, _ := ExecuteSmartCascade(ctx, []InferenceEngine{weak}, req, nil, DefaultSmartCascadeConfig())
		if report.SmartFailuresCounted != 1 || report.TrialFailuresIgnored != 0 {
			t.Fatalf("report = %+v", report)
		}
	})
}

func TestSmartCascadeEscalatesAfterTheLimitOfSmartFailures(t *testing.T) {
	boom := errors.New("boom")
	a := &scriptedEngine{id: "a", typ: EngineOllama, models: []string{"qwen3:32b"}, selfHost: true, failWith: boom}
	b := &scriptedEngine{id: "b", typ: EngineVLLM, models: []string{"qwen3:32b"}, selfHost: true, failWith: boom}
	c := &scriptedEngine{id: "c", typ: EngineLlamaCpp, models: []string{"qwen3:32b"}, selfHost: true, failWith: boom}
	paid := func(_ context.Context, _ CompletionRequest, model string) (*CompletionResponse, error) {
		return &CompletionResponse{Model: model}, nil
	}
	_, report, err := ExecuteSmartCascade(context.Background(), []InferenceEngine{c, b, a}, CompletionRequest{Prompt: "hi"}, paid, SmartCascadeConfig{MaxSmartFailures: 2})
	if err != nil || !report.LeveragedToPaid {
		t.Fatalf("report = %+v, err = %v", report, err)
	}
	if report.Attempts != 2 || len(c.calls) != 0 {
		t.Errorf("the cascade stops at the limit: attempts = %d, third engine called %d times", report.Attempts, len(c.calls))
	}
	if report.SelectedEngine != "paid-cloud-orchestrator" {
		t.Errorf("selected = %q", report.SelectedEngine)
	}
}
