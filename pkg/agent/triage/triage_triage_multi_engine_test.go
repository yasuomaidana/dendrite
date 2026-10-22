package triage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/inference"
	"Dendrite/pkg/ollama"
)

func TestTriageMultiEngineSupport(t *testing.T) {
	engine := NewEngine(nil)
	computeTrue := true
	engine.MockComputeAvailable = &computeTrue

	tests := []struct {
		name           string
		targetEngine   SupportedEngine
		expectedModel  string
		expectedEngine string
	}{
		{
			name:           "Ollama Engine",
			targetEngine:   EngineOllama,
			expectedModel:  "local-qwen-coder",
			expectedEngine: "Ollama Metal GPU",
		},
		{
			name:           "Ollama Validated Engine",
			targetEngine:   EngineOllamaValidated,
			expectedModel:  "qwen2.5-coder:14b",
			expectedEngine: "Ollama (JetBrains Validated)",
		},
		{
			name:           "Hugging Face Engine",
			targetEngine:   EngineHuggingFace,
			expectedModel:  "deepseek-ai/DeepSeek-Coder-V2-Lite-Instruct",
			expectedEngine: "Hugging Face TGI/TEI Inference Engine",
		},
		{
			name:           "OxiLLaMa Pure-Rust Engine (cool-japan/oxillama)",
			targetEngine:   EngineOxiLLaMa,
			expectedModel:  "oxillama-rust-qwen7b",
			expectedEngine: "OxiLLaMa Pure-Rust Sovereign Engine",
		},
		{
			name:           "Candle Rust Framework Engine (huggingface/candle)",
			targetEngine:   EngineCandle,
			expectedModel:  "candle-quant-llama",
			expectedEngine: "Candle Rust Framework Engine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := TriageRequest{
				Prompt:       "Fix formatting and explain linter error",
				TargetEngine: tt.targetEngine,
				ExecuteLocal: true,
			}
			res, err := engine.Triage(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Tier != TierLocal {
				t.Errorf("expected tier local, got %s", res.Tier)
			}
			if res.AssignedModel != tt.expectedModel {
				t.Errorf("expected model %s, got %s", tt.expectedModel, res.AssignedModel)
			}
			if !strings.Contains(res.LocalEngineName, tt.expectedEngine) {
				t.Errorf("expected engine %s, got %s", tt.expectedEngine, res.LocalEngineName)
			}
			if res.TokenSavingsPct != 100.0 {
				t.Errorf("expected 100%% token savings, got %f", res.TokenSavingsPct)
			}
		})
	}

	// Explicitly verify OxiLLaMa and Candle are different engines
	reqOxillama := TriageRequest{Prompt: "Fix formatting", TargetEngine: EngineOxiLLaMa}
	resOxillama, _ := engine.Triage(context.Background(), reqOxillama)

	reqCandle := TriageRequest{Prompt: "Fix formatting", TargetEngine: EngineCandle}
	resCandle, _ := engine.Triage(context.Background(), reqCandle)

	if resOxillama.AssignedEngine == resCandle.AssignedEngine {
		t.Errorf("OxiLLaMa and Candle must be distinct engines! Got %s for both", resOxillama.AssignedEngine)
	}
	if resOxillama.AssignedModel == resCandle.AssignedModel {
		t.Errorf("OxiLLaMa and Candle models must be distinct! Got %s for both", resOxillama.AssignedModel)
	}
}

func TestTriageDeclarativeOrchestrator(t *testing.T) {
	engine := NewEngine(nil)
	computeTrue := true
	engine.MockComputeAvailable = &computeTrue

	req := TriageRequest{
		Prompt:                        "Implement high-throughput persistent ring buffer with ticket DEN-050 and check cyclomatic complexity",
		ActiveFile:                    "pkg/buffer/ring.go",
		EnableDeclarativeOrchestrator: true,
		ExecuteLocal:                  true,
		TargetEngine:                  EngineOllamaValidated,
	}

	res, err := engine.Triage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected triage error: %v", err)
	}

	if !res.IsDeclarativeOrchestrator {
		t.Fatalf("expected IsDeclarativeOrchestrator to be true when local intelligent models are available")
	}

	if res.OrchestratorModel != "copilot-sonnet" && res.OrchestratorModel != "gemini-2.5-pro" {
		t.Errorf("expected paid orchestrator model, got: %s", res.OrchestratorModel)
	}

	if res.WorkerModel != "qwen2.5-coder:14b" {
		t.Errorf("expected worker model qwen2.5-coder:14b, got: %s", res.WorkerModel)
	}

	if res.DeclarativePlan == nil {
		t.Fatalf("expected declarative plan to be generated")
	}

	plan := res.DeclarativePlan
	if len(plan.Instructions) == 0 {
		t.Errorf("expected declarative plan instructions, got empty")
	}

	t.Logf("Declarative Instructions: %v", plan.Instructions)
	t.Logf("Declarative Constraints: %v", plan.Constraints)
	t.Logf("Estimated Orchestrator Tokens: %d", plan.EstimatedOrchestratorTokens)
	t.Logf("Estimated Direct Output Tokens: %d", plan.EstimatedDirectOutputTokens)
	t.Logf("Output Token Savings: %.1f%%", plan.OutputTokenSavingsPct)

	if plan.OutputTokenSavingsPct < 80.0 {
		t.Errorf("expected output token savings >= 80%% in declarative orchestrator mode, got %.1f%%", plan.OutputTokenSavingsPct)
	}

	if !strings.Contains(res.LocalWorkerExecutionResult, "Executed") {
		t.Errorf("expected local worker execution result, got: %s", res.LocalWorkerExecutionResult)
	}
}

func TestTriage_OllamaPrecedenceOverSmarterOxiLLaMaModel(t *testing.T) {
	// Ollama test server that returns only a small, less-smart model (llama3.2:3b)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models":[{"name":"llama3.2:3b","model":"llama3.2:3b"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	tsOxillama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[{"id":"oxillama-rust-qwen7b"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer tsOxillama.Close()

	ollamaClient := ollama.NewClient(ollama.WithBaseURL(ts.URL))

	engine := NewEngine(ollamaClient)
	engine.OxiLLaMaEngine = inference.NewOxiLLaMaEngine(inference.EngineConfig{
		ID:       "oxillama-rust",
		Type:     inference.EngineOxiLLaMa,
		Endpoint: tsOxillama.URL,
	}, tsOxillama.Client())

	req := TriageRequest{
		Prompt: "Fix formatting and add docstrings for this struct",
	}

	res, err := engine.Triage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected triage error: %v", err)
	}

	// OxiLLaMa hosts a smarter 7b model, but it prefills on the CPU: Ollama (GPU) wins.
	if res.AssignedEngine != EngineOllama {
		t.Errorf("expected Ollama (GPU) to take precedence over OxiLLaMa, got %s", res.AssignedEngine)
	}
	if res.AssignedModel != "llama3.2:3b" {
		t.Errorf("expected assigned model 'llama3.2:3b', got %s", res.AssignedModel)
	}

	// With no Ollama model, OxiLLaMa serves and the reason says it runs on the CPU.
	tsEmpty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models":[]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer tsEmpty.Close()
	engine.OllamaClient = ollama.NewClient(ollama.WithBaseURL(tsEmpty.URL))
	res, err = engine.Triage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected triage error: %v", err)
	}
	if res.AssignedEngine != EngineOxiLLaMa || res.AssignedModel != "oxillama-rust-qwen7b" {
		t.Errorf("expected OxiLLaMa oxillama-rust-qwen7b without Ollama models, got %s %s", res.AssignedEngine, res.AssignedModel)
	}
	if !strings.Contains(res.Reason, "CPU") {
		t.Errorf("expected reason to note OxiLLaMa runs on the CPU, got: %s", res.Reason)
	}
}

func TestTriage_DynamicProbingLiveOxiLLaMaAndOllama(t *testing.T) {
	// 1. Case: OxiLLaMa has qwen3.8:27b (score 96), Ollama has hermes3:8b (score 67)
	tsOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models":[{"name":"hermes3:8b","model":"hermes3:8b"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer tsOllama.Close()

	tsOxillama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[{"id":"qwen3.8:27b"},{"id":"qwen3.5:9b"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer tsOxillama.Close()

	ollamaClient := ollama.NewClient(ollama.WithBaseURL(tsOllama.URL))
	oxEng := inference.NewOxiLLaMaEngine(inference.EngineConfig{
		ID:       "oxillama-rust",
		Type:     inference.EngineOxiLLaMa,
		Endpoint: tsOxillama.URL,
	}, tsOxillama.Client())

	engine := NewEngine(ollamaClient)
	engine.OxiLLaMaEngine = oxEng

	req := TriageRequest{Prompt: "Fix formatting and lint error"}
	res, err := engine.Triage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected triage error: %v", err)
	}

	// Ollama (GPU) wins although OxiLLaMa hosts the smarter model.
	if res.AssignedEngine != EngineOllama {
		t.Errorf("expected Ollama to win over OxiLLaMa, got %s", res.AssignedEngine)
	}
	if res.AssignedModel != "hermes3:8b" {
		t.Errorf("expected assigned model hermes3:8b, got %s", res.AssignedModel)
	}

	// 2. Case: Ollama has qwen2.5-coder:32b (score 100), OxiLLaMa has qwen3.5:9b (score 73)
	tsOllama2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models":[{"name":"qwen2.5-coder:32b","model":"qwen2.5-coder:32b"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer tsOllama2.Close()

	tsOxillama2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[{"id":"qwen3.5:9b"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer tsOxillama2.Close()

	engine2 := NewEngine(ollama.NewClient(ollama.WithBaseURL(tsOllama2.URL)))
	engine2.OxiLLaMaEngine = inference.NewOxiLLaMaEngine(inference.EngineConfig{
		ID:       "oxillama-rust",
		Type:     inference.EngineOxiLLaMa,
		Endpoint: tsOxillama2.URL,
	}, tsOxillama2.Client())

	res2, err2 := engine2.Triage(context.Background(), req)
	if err2 != nil {
		t.Fatalf("unexpected triage error: %v", err2)
	}
	if res2.AssignedEngine != EngineOllama {
		t.Errorf("expected EngineOllama to win with 32B model, got %s", res2.AssignedEngine)
	}
	if res2.AssignedModel != "qwen2.5-coder:32b" {
		t.Errorf("expected assigned model qwen2.5-coder:32b, got %s", res2.AssignedModel)
	}
}

func TestTriage_RAGContextInjectionAndDeclarativeGrounding(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create a markdown doc with architectural guidelines
	docPath := filepath.Join(tmpDir, "storage-arch.md")
	docContent := `# Storage Subsystem Architecture
The persistent storage engine must utilize atomic fsync commits and maintain bounded memory buffers.
`
	if err := os.WriteFile(docPath, []byte(docContent), 0644); err != nil {
		t.Fatalf("failed to write doc: %v", err)
	}

	// 2. Create a Go source file
	codePath := filepath.Join(tmpDir, "buffer.go")
	codeContent := `package storage

// RingBuffer manages circular in-memory segments.
type RingBuffer struct {
	Capacity int
	Head     int
}

// Push appends an item to the ring buffer.
func (r *RingBuffer) Push(val int) bool {
	if r.Capacity <= 0 {
		return false
	}
	r.Head = (r.Head + 1) % r.Capacity
	return true
}
`
	if err := os.WriteFile(codePath, []byte(codeContent), 0644); err != nil {
		t.Fatalf("failed to write code: %v", err)
	}

	engine := NewEngine(nil)
	computeTrue := true
	engine.MockComputeAvailable = &computeTrue

	req := TriageRequest{
		Prompt:       "Implement persistent ring buffer and evaluate cyclomatic complexity",
		WorkspaceDir: tmpDir,
		EnableRAG:    true,
		RAGTopK:      2,
	}

	res, err := engine.Triage(context.Background(), req)
	if err != nil {
		t.Fatalf("Triage with RAG failed: %v", err)
	}

	if len(res.RetrievedRAGChunks) == 0 {
		t.Fatalf("expected retrieved RAG chunks, got 0")
	}

	if !strings.Contains(res.EnhancedPrompt, "=== RETRIEVED ARCHITECTURE & CODE CONTEXT (AST-Guided RAG) ===") {
		t.Errorf("expected EnhancedPrompt to contain RAG context section, got: %s", res.EnhancedPrompt)
	}

	if res.DeclarativePlan == nil {
		t.Fatalf("expected DeclarativePlan to be synthesized")
	}

	// Check declarative plan target files ground the retrieved file
	foundTarget := false
	for _, tf := range res.DeclarativePlan.TargetFiles {
		if strings.Contains(tf, "buffer.go") || strings.Contains(tf, "storage-arch.md") {
			foundTarget = true
			break
		}
	}
	if !foundTarget {
		t.Errorf("expected DeclarativePlan.TargetFiles to contain RAG files, got: %+v", res.DeclarativePlan.TargetFiles)
	}
}
