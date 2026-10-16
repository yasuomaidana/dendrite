package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/inference"
)

// testEngine implements inference.InferenceEngine for zero-mock testing.
type testEngine struct {
	id         string
	engineType inference.EngineType
	models     []string
	generateFn func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error)
	healthFn   func(ctx context.Context) inference.EngineHealth
}

func (m *testEngine) ID() string                 { return m.id }
func (m *testEngine) Type() inference.EngineType { return m.engineType }
func (m *testEngine) Config() inference.EngineConfig {
	return inference.EngineConfig{
		ID:         m.id,
		Type:       m.engineType,
		Models:     m.models,
		SelfHosted: true,
	}
}
func (m *testEngine) IsSelfHosted() bool { return true }
func (m *testEngine) Generate(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
	if m.generateFn != nil {
		return m.generateFn(ctx, req)
	}
	return &inference.CompletionResponse{
		ID:       "test-resp",
		Model:    req.Model,
		EngineID: m.id,
		Text:     "To check available LLMs, use: dendrite inference list",
	}, nil
}
func (m *testEngine) Stream(ctx context.Context, req inference.CompletionRequest) (<-chan inference.StreamChunk, error) {
	return nil, nil
}
func (m *testEngine) Health(ctx context.Context) inference.EngineHealth {
	if m.healthFn != nil {
		return m.healthFn(ctx)
	}
	return inference.EngineHealth{EngineID: m.id, Reachable: true}
}
func (m *testEngine) SupportedModels(ctx context.Context) ([]string, error) {
	return m.models, nil
}

func TestAsk_EmptyQuestion(t *testing.T) {
	engine := NewAskEngine(nil)
	_, err := engine.Ask(context.Background(), AskRequest{
		Question: "   ",
	})
	if !errors.Is(err, ErrEmptyQuestion) {
		t.Fatalf("expected ErrEmptyQuestion, got: %v", err)
	}
}

func TestAsk_LocalLLM_Success(t *testing.T) {
	tmpDir := t.TempDir()
	docFile := filepath.Join(tmpDir, "README.md")
	_ = os.WriteFile(docFile, []byte("# Dendrite\nRun `dendrite inference list` to check available LLMs.\n"), 0644)

	localEng := &testEngine{
		id:         "ollama-metal",
		engineType: inference.EngineOllama,
		models:     []string{"qwen2.5-coder:14b"},
		generateFn: func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
			return &inference.CompletionResponse{
				ID:           "resp-local",
				Model:        "qwen2.5-coder:14b",
				EngineID:     "ollama-metal",
				Text:         "You can check available LLMs using `dendrite inference list` and `dendrite dev inference zero-cost list`.",
				PromptTokens: 120,
				OutputTokens: 35,
			}, nil
		},
	}

	engine := NewAskEngine(nil)
	engine.LocalEngine = localEng

	var out bytes.Buffer
	res, err := engine.Ask(context.Background(), AskRequest{
		Question:    "I want to know how to check available LLMs",
		ProjectPath: tmpDir,
		Output:      &out,
	})
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}

	if res.SourceTier != "local-rag" {
		t.Errorf("expected SourceTier local-rag, got: %s", res.SourceTier)
	}
	if res.Escalated {
		t.Errorf("expected Escalated to be false")
	}
	if !strings.Contains(res.Answer, "dendrite inference list") {
		t.Errorf("expected answer to contain 'dendrite inference list', got: %s", res.Answer)
	}
	if len(res.RetrievedChunks) == 0 {
		t.Errorf("expected retrieved chunks to be populated")
	}
}

func TestAsk_HybridMode_Success(t *testing.T) {
	tmpDir := t.TempDir()
	docFile := filepath.Join(tmpDir, "README.md")
	_ = os.WriteFile(docFile, []byte("# LLM Architecture\nUse `dendrite dev inference list` to check available LLMs.\n"), 0644)

	localEng := &testEngine{
		id:         "ollama-metal",
		engineType: inference.EngineOllama,
		models:     []string{"hermes3:8b"},
		generateFn: func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
			return &inference.CompletionResponse{
				ID:           "local-draft-id",
				Model:        "hermes3:8b",
				EngineID:     "ollama-metal",
				Text:         "Draft answer: list models via dendrite inference list.",
				PromptTokens: 100,
				OutputTokens: 20,
			}, nil
		},
	}

	paidCalled := false
	paidCaller := func(ctx context.Context, prompt string, paidModel string) (*inference.CompletionResponse, error) {
		paidCalled = true
		if !strings.Contains(prompt, "Initial Local Draft") {
			t.Errorf("expected hybrid refinement prompt to include Initial Local Draft")
		}
		return &inference.CompletionResponse{
			ID:           "paid-refined-id",
			Model:        paidModel,
			EngineID:     "paid-cloud-orchestrator",
			Text:         "Refined Answer: To inspect available LLMs in Dendrite, execute `dendrite dev inference list`.",
			PromptTokens: 250,
			OutputTokens: 30,
		}, nil
	}

	engine := NewAskEngine(nil)
	engine.LocalEngine = localEng
	engine.PaidCaller = paidCaller

	var out bytes.Buffer
	res, err := engine.Ask(context.Background(), AskRequest{
		Question:    "I want to know how to check available LLMs",
		ProjectPath: tmpDir,
		HybridMode:  true,
		PaidModel:   "gemini-2.5-pro",
		Output:      &out,
	})
	if err != nil {
		t.Fatalf("Hybrid Ask failed: %v", err)
	}

	if !paidCalled {
		t.Errorf("expected paid caller to be invoked for refinement in hybrid mode")
	}
	if res.SourceTier != "hybrid" {
		t.Errorf("expected SourceTier 'hybrid', got: %s", res.SourceTier)
	}
	if res.LocalDraft == "" {
		t.Errorf("expected LocalDraft to be populated in hybrid mode")
	}
	if !strings.Contains(res.Answer, "Refined Answer") {
		t.Errorf("expected refined answer in res.Answer, got: %s", res.Answer)
	}
	if !strings.Contains(out.String(), "Hybrid mode: Local draft generated") {
		t.Errorf("expected hybrid mode output message, got: %s", out.String())
	}
}

func TestAsk_HybridMode_PaidRefinementFails_FallbackLocalDraft(t *testing.T) {
	tmpDir := t.TempDir()
	localEng := &testEngine{
		id:         "ollama-metal",
		engineType: inference.EngineOllama,
		models:     []string{"hermes3:8b"},
		generateFn: func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
			return &inference.CompletionResponse{
				ID:       "local-draft-id",
				Model:    "hermes3:8b",
				EngineID: "ollama-metal",
				Text:     "Local draft answer without refinement.",
			}, nil
		},
	}

	engine := NewAskEngine(nil)
	engine.LocalEngine = localEng
	engine.PaidCaller = func(ctx context.Context, prompt string, paidModel string) (*inference.CompletionResponse, error) {
		return nil, errors.New("network timeout contacting paid cloud API")
	}

	var out bytes.Buffer
	res, err := engine.Ask(context.Background(), AskRequest{
		Question:    "How does the hybrid mode work?",
		ProjectPath: tmpDir,
		HybridMode:  true,
		PaidModel:   "gemini-2.5-pro",
		Output:      &out,
	})
	if err != nil {
		t.Fatalf("expected fallback to local draft on paid refinement failure, got err: %v", err)
	}

	if res.SourceTier != "local-rag" {
		t.Errorf("expected SourceTier fallback to local-rag, got: %s", res.SourceTier)
	}
	if !strings.Contains(res.Answer, "Local draft answer") {
		t.Errorf("expected local draft in answer, got: %s", res.Answer)
	}
	if !strings.Contains(out.String(), "Paid refinement failed") {
		t.Errorf("expected warning message in output, got: %s", out.String())
	}
}

func TestAsk_HybridMode_LocalFails_Escalates(t *testing.T) {
	tmpDir := t.TempDir()
	localEng := &testEngine{
		id:         "ollama-metal",
		engineType: inference.EngineOllama,
		generateFn: func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
			return nil, errors.New("ollama offline")
		},
	}

	engine := NewAskEngine(nil)
	engine.LocalEngine = localEng
	engine.PaidCaller = func(ctx context.Context, prompt string, paidModel string) (*inference.CompletionResponse, error) {
		return &inference.CompletionResponse{
			ID:       "paid-esc-id",
			Model:    paidModel,
			EngineID: "paid-cloud-orchestrator",
			Text:     "Paid cloud response after local failure in hybrid mode.",
		}, nil
	}

	var out bytes.Buffer
	res, err := engine.Ask(context.Background(), AskRequest{
		Question:        "Explain hybrid fallback",
		ProjectPath:     tmpDir,
		HybridMode:      true,
		AutoApprovePaid: true,
		PaidModel:       "gemini-2.5-pro",
		Output:          &out,
	})
	if err != nil {
		t.Fatalf("expected escalation in hybrid mode when local fails, got: %v", err)
	}

	if res.SourceTier != "paid-cloud" {
		t.Errorf("expected SourceTier paid-cloud, got: %s", res.SourceTier)
	}
}

func TestAsk_CentralRAG_Delegation(t *testing.T) {
	// Mock Central RAG daemon
	centralServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rag/ask" {
			http.NotFound(w, r)
			return
		}
		var req AskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		res := AskResult{
			Question:   req.Question,
			Answer:     "Central RAG Response: Shared environment coordination active.",
			SourceTier: "central-rag",
			Model:      "shared-qwen-72b",
			Engine:     "central-daemon",
			RetrievedChunks: []Chunk{
				{FilePath: "shared/docs/architecture.md", Content: "Central RAG coordinator"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer centralServer.Close()

	engine := NewAskEngine(nil)
	res, err := engine.Ask(context.Background(), AskRequest{
		Question:  "Where does the RAG service live in shared environments?",
		ServerURL: centralServer.URL,
	})
	if err != nil {
		t.Fatalf("central RAG query failed: %v", err)
	}

	if res.SourceTier != "central-rag" {
		t.Errorf("expected SourceTier 'central-rag', got: %s", res.SourceTier)
	}
	if !strings.Contains(res.Answer, "Central RAG Response") {
		t.Errorf("unexpected answer: %s", res.Answer)
	}
}

func TestAsk_CentralRAG_ErrorResponse(t *testing.T) {
	centralServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "central index unavailable", http.StatusInternalServerError)
	}))
	defer centralServer.Close()

	engine := NewAskEngine(nil)
	_, err := engine.Ask(context.Background(), AskRequest{
		Question:  "Query central server",
		ServerURL: centralServer.URL,
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected HTTP 500 error from central RAG, got: %v", err)
	}
}

func TestAsk_LocalFails_HumanDeclined(t *testing.T) {
	tmpDir := t.TempDir()
	localEng := &testEngine{
		id:         "ollama-metal",
		engineType: inference.EngineOllama,
		generateFn: func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
			return nil, errors.New("connection refused: Ollama daemon is offline")
		},
	}

	engine := NewAskEngine(nil)
	engine.LocalEngine = localEng

	var out bytes.Buffer
	in := strings.NewReader("n\n") // Human says 'n'

	_, err := engine.Ask(context.Background(), AskRequest{
		Question:    "I want to know how to check available LLMs",
		ProjectPath: tmpDir,
		Input:       in,
		Output:      &out,
	})

	if err == nil {
		t.Fatalf("expected error when human declines escalation")
	}
	if !errors.Is(err, ErrPaidEscalationDeclined) {
		t.Errorf("expected ErrPaidEscalationDeclined, got: %v", err)
	}
	if !strings.Contains(out.String(), "Paid LLM escalation declined by user") {
		t.Errorf("expected cancellation message in output, got: %s", out.String())
	}
}

func TestAsk_LocalFails_HumanDeclinedEOF(t *testing.T) {
	tmpDir := t.TempDir()
	localEng := &testEngine{
		id:         "ollama-metal",
		engineType: inference.EngineOllama,
		generateFn: func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
			return nil, errors.New("timeout")
		},
	}

	engine := NewAskEngine(nil)
	engine.LocalEngine = localEng

	var out bytes.Buffer
	in := strings.NewReader("") // EOF immediately

	_, err := engine.Ask(context.Background(), AskRequest{
		Question:    "I want to know how to check available LLMs",
		ProjectPath: tmpDir,
		Input:       in,
		Output:      &out,
	})

	if err == nil {
		t.Fatalf("expected error when input is EOF")
	}
	if !errors.Is(err, ErrPaidEscalationDeclined) {
		t.Errorf("expected ErrPaidEscalationDeclined on EOF, got: %v", err)
	}
}

func TestAsk_LocalFails_HumanAccepted(t *testing.T) {
	tmpDir := t.TempDir()
	localEng := &testEngine{
		id:         "ollama-metal",
		engineType: inference.EngineOllama,
		generateFn: func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
			return nil, errors.New("Ollama socket closed")
		},
	}

	engine := NewAskEngine(nil)
	engine.LocalEngine = localEng
	engine.PaidCaller = func(ctx context.Context, prompt string, paidModel string) (*inference.CompletionResponse, error) {
		return &inference.CompletionResponse{
			ID:           "paid-resp-1",
			Model:        paidModel,
			EngineID:     "paid-cloud-orchestrator",
			Text:         "To inspect available LLMs in Dendrite, execute `dendrite dev inference list` or `dendrite inference list`.",
			PromptTokens: 250,
			OutputTokens: 40,
		}, nil
	}

	var out bytes.Buffer
	in := strings.NewReader("yes\n") // Human confirms 'yes'

	res, err := engine.Ask(context.Background(), AskRequest{
		Question:    "I want to know how to check available LLMs",
		ProjectPath: tmpDir,
		PaidModel:   "gemini-2.5-pro",
		Input:       in,
		Output:      &out,
	})

	if err != nil {
		t.Fatalf("expected success on accepted escalation, got: %v", err)
	}
	if res.SourceTier != "paid-cloud" {
		t.Errorf("expected SourceTier paid-cloud, got: %s", res.SourceTier)
	}
	if !res.Escalated || !res.EscalationApproved {
		t.Errorf("expected Escalated and EscalationApproved to be true")
	}
	if res.Model != "gemini-2.5-pro" {
		t.Errorf("expected model gemini-2.5-pro, got: %s", res.Model)
	}
	if !strings.Contains(res.Answer, "dendrite inference list") {
		t.Errorf("expected answer to contain 'dendrite inference list', got: %s", res.Answer)
	}
}

func TestAsk_LocalFails_AutoApproveFlag(t *testing.T) {
	tmpDir := t.TempDir()
	localEng := &testEngine{
		id:         "oxillama-rust",
		engineType: inference.EngineOxiLLaMa,
		generateFn: func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
			return nil, errors.New("OxiLLaMa model not loaded")
		},
	}

	engine := NewAskEngine(nil)
	engine.LocalEngine = localEng

	paidCalled := false
	engine.PaidCaller = func(ctx context.Context, prompt string, paidModel string) (*inference.CompletionResponse, error) {
		paidCalled = true
		return &inference.CompletionResponse{
			ID:           "paid-resp-auto",
			Model:        paidModel,
			EngineID:     "paid-cloud-orchestrator",
			Text:         "Execute `dendrite inference list` to see all configured LLMs.",
			PromptTokens: 180,
			OutputTokens: 25,
		}, nil
	}

	var out bytes.Buffer
	// With AutoApprovePaid = true (-y flag), input should never be read!
	res, err := engine.Ask(context.Background(), AskRequest{
		Question:        "I want to know how to check available LLMs",
		ProjectPath:     tmpDir,
		AutoApprovePaid: true, // -y flag
		PaidModel:       "gemini-2.5-pro",
		Output:          &out,
	})

	if err != nil {
		t.Fatalf("expected success with -y flag, got: %v", err)
	}
	if !paidCalled {
		t.Errorf("expected paid caller to be invoked")
	}
	if res.SourceTier != "paid-cloud" {
		t.Errorf("expected SourceTier paid-cloud, got: %s", res.SourceTier)
	}
	if !strings.Contains(out.String(), "Flag -y passed: automatically escalating") {
		t.Errorf("expected auto-escalating message in output, got: %s", out.String())
	}
}

func TestAsk_DefaultPaidCaller_HTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "bad method", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-master-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		resp := map[string]interface{}{
			"id": "chatcmpl-test",
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": "You can check available LLMs with `dendrite inference list`.",
					},
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     150,
				"completion_tokens": 30,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	os.Setenv("LITELLM_PROXY_URL", server.URL)
	os.Setenv("LITELLM_MASTER_KEY", "test-master-key")
	defer os.Unsetenv("LITELLM_PROXY_URL")
	defer os.Unsetenv("LITELLM_MASTER_KEY")

	engine := NewAskEngine(nil)
	resp, err := engine.defaultPaidCaller(context.Background(), "How to check available LLMs", "gemini-2.5-pro")
	if err != nil {
		t.Fatalf("defaultPaidCaller failed: %v", err)
	}
	if resp.Text != "You can check available LLMs with `dendrite inference list`." {
		t.Errorf("unexpected text: %s", resp.Text)
	}
}

func TestAsk_DefaultPaidCaller_ErrorHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "quota exceeded", http.StatusTooManyRequests)
	}))
	defer server.Close()

	os.Setenv("LITELLM_PROXY_URL", server.URL)
	defer os.Unsetenv("LITELLM_PROXY_URL")

	engine := NewAskEngine(nil)
	_, err := engine.defaultPaidCaller(context.Background(), "test prompt", "gemini-2.5-pro")
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("expected HTTP 429 error, got: %v", err)
	}
}

func TestAsk_PaidCaller_ErrorHandling(t *testing.T) {
	engine := NewAskEngine(nil)
	engine.LocalEngine = &testEngine{
		id: "err-eng",
		generateFn: func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
			return nil, errors.New("local failure")
		},
	}
	engine.PaidCaller = func(ctx context.Context, prompt string, paidModel string) (*inference.CompletionResponse, error) {
		return nil, errors.New("upstream quota exceeded")
	}

	var out bytes.Buffer
	_, err := engine.Ask(context.Background(), AskRequest{
		Question:        "test question",
		AutoApprovePaid: true,
		Output:          &out,
	})
	if err == nil || !strings.Contains(err.Error(), "upstream quota exceeded") {
		t.Fatalf("expected paid LLM failure error, got: %v", err)
	}
}

func TestAsk_TryLocalLLM_Manager(t *testing.T) {
	mgr := inference.NewManager()
	eng := &testEngine{
		id:         "local-mock",
		engineType: inference.EngineOllama,
		models:     []string{"qwen2.5-coder:14b"},
		generateFn: func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
			return &inference.CompletionResponse{
				ID:   "mgr-resp",
				Text: "Response from manager",
			}, nil
		},
	}
	_ = mgr.RegisterEngine(eng)

	engine := NewAskEngine(mgr)
	resp, err := engine.tryLocalLLM(context.Background(), AskRequest{}, "sys", "prompt")
	if err != nil {
		t.Fatalf("tryLocalLLM failed: %v", err)
	}
	if resp.Text != "Response from manager" {
		t.Errorf("unexpected manager text: %s", resp.Text)
	}
}

func TestAsk_ForcePaid_DirectEscalation(t *testing.T) {
	engine := NewAskEngine(nil)
	engine.LocalEngine = &testEngine{
		id: "local-stub",
		generateFn: func(ctx context.Context, req inference.CompletionRequest) (*inference.CompletionResponse, error) {
			t.Fatal("local engine should NOT be called when ForcePaid is true")
			return nil, nil
		},
	}

	paidCalled := false
	engine.PaidCaller = func(ctx context.Context, prompt string, paidModel string) (*inference.CompletionResponse, error) {
		paidCalled = true
		return &inference.CompletionResponse{
			ID:       "paid-resp-force",
			Model:    paidModel,
			EngineID: "paid-cloud-orchestrator",
			Text:     "Detailed answer from paid cloud model.",
		}, nil
	}

	var out bytes.Buffer
	res, err := engine.Ask(context.Background(), AskRequest{
		Question:  "How to check available LLMs?",
		ForcePaid: true,
		PaidModel: "gemini-2.5-pro",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("expected success with ForcePaid, got: %v", err)
	}
	if !paidCalled {
		t.Errorf("expected paid caller to be invoked")
	}
	if res.SourceTier != "paid-cloud" {
		t.Errorf("expected SourceTier paid-cloud, got: %s", res.SourceTier)
	}
	if !strings.Contains(out.String(), "Force-paid flag enabled") {
		t.Errorf("expected force-paid notification in output, got: %s", out.String())
	}
}

func TestEnhanceAnswer_Cases(t *testing.T) {
	chunks := []Chunk{
		{
			FilePath: "docs/guide.md",
			Content:  "Run the following command:\n```bash\ndendrite inference list\n```\nTo inspect models.",
		},
	}

	// 1. Canned response with LLM question
	ans1 := enhanceAnswer("check available llms", "processed your request safely with zero cloud tokens consumed", chunks)
	if !strings.Contains(ans1, "dendrite inference list") {
		t.Errorf("expected ans1 to contain dendrite inference list, got: %s", ans1)
	}

	// 2. Canned response with general command question
	ans2 := enhanceAnswer("show me the command to build", "short", chunks)
	if !strings.Contains(ans2, "dendrite inference list") {
		t.Errorf("expected ans2 to contain extracted commands, got: %s", ans2)
	}

	// 3. User specifically requests commands, but answer has no command
	ans3 := enhanceAnswer("show me the command", "You can easily inspect models in the CLI.", chunks)
	if !strings.Contains(ans3, "Key Commands from Project Documentation") {
		t.Errorf("expected Key Commands section in ans3, got: %s", ans3)
	}

	// 4. Normal answer that already includes command
	ans4 := enhanceAnswer("how to list", "Execute dendrite inference list to see models.", chunks)
	if ans4 != "Execute dendrite inference list to see models." {
		t.Errorf("unexpected change to ans4: %s", ans4)
	}
}
