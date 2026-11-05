package triage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"Dendrite/pkg/inference"
	"Dendrite/pkg/ollama"
	"Dendrite/pkg/rag"
)

func TestNewEngineVariants(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	t.Run("with explicit client", func(t *testing.T) {
		client := ollama.NewClient(ollama.WithBaseURL(ts.URL))
		eng := NewEngine(client)
		if eng.OllamaClient != client {
			t.Errorf("expected client to be preserved")
		}
	})

	t.Run("with empty endpoint env", func(t *testing.T) {
		t.Setenv("DENDRITE_OLLAMA_ENDPOINT", "")
		eng := NewEngine(nil)
		if eng.OllamaClient == nil {
			t.Errorf("expected default ollama client to be constructed")
		}
		if eng.OllamaEndpoint != "" {
			t.Errorf("expected empty endpoint, got %q", eng.OllamaEndpoint)
		}
	})
}

func TestTargetEngineRouting(t *testing.T) {
	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5-coder:14b","model":"qwen2.5-coder:14b"}]}`))
			return
		}
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"oxillama-rust-qwen7b"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer liveServer.Close()
	t.Setenv("DENDRITE_OLLAMA_ENDPOINT", liveServer.URL)
	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", liveServer.URL)

	cases := []struct {
		name       string
		engine     SupportedEngine
		tier       ExecutionTier
		wantEngine SupportedEngine
		wantModel  string
	}{
		{"validated engine", EngineOllamaValidated, TierLocal, EngineOllamaValidated, "qwen2.5-coder:14b"},
		{"huggingface engine", EngineHuggingFace, TierLocal, EngineHuggingFace, "deepseek-ai/DeepSeek-Coder-V2-Lite-Instruct"},
		{"oxillama engine", EngineOxiLLaMa, TierLocal, EngineOxiLLaMa, "oxillama-rust-qwen7b"},
		{"candle engine", EngineCandle, TierLocal, EngineCandle, "candle-quant-llama"},
		{"ollama engine", EngineOllama, TierLocal, EngineOllama, "qwen2.5-coder:14b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng := NewEngine(nil)
			res, err := eng.Triage(context.Background(), TriageRequest{
				Prompt:       "Fix formatting",
				TargetEngine: tc.engine,
				ForceTier:    tc.tier,
			})
			if err != nil {
				t.Fatalf("triage failed: %v", err)
			}
			if res.AssignedEngine != tc.wantEngine {
				t.Errorf("got engine %v, want %v", res.AssignedEngine, tc.wantEngine)
			}
			if res.AssignedModel != tc.wantModel {
				t.Errorf("got model %v, want %v", res.AssignedModel, tc.wantModel)
			}
		})
	}
}

func TestAutoRoutingOllamaOfflineOxiLive(t *testing.T) {
	offlineOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "offline", http.StatusInternalServerError)
	}))
	defer offlineOllama.Close()

	onlineOxillama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"oxillama-rust-qwen7b"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer onlineOxillama.Close()

	t.Setenv("DENDRITE_OLLAMA_ENDPOINT", offlineOllama.URL)
	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", onlineOxillama.URL)

	eng := NewEngine(nil)

	t.Run("chore routes to oxillama with reason update", func(t *testing.T) {
		res, err := eng.Triage(context.Background(), TriageRequest{
			Prompt: "Fix formatting and typo in comments",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.AssignedEngine != EngineOxiLLaMa {
			t.Errorf("expected EngineOxiLLaMa, got %v", res.AssignedEngine)
		}
		if !strings.Contains(res.Reason, "Ollama has no model") {
			t.Errorf("expected reason to mention Ollama has no model, got %s", res.Reason)
		}
		if !res.LocalComputeAvailable {
			t.Errorf("expected LocalComputeAvailable true")
		}
	})

	t.Run("hybrid prompt routes to oxillama without chore reason", func(t *testing.T) {
		res, err := eng.Triage(context.Background(), TriageRequest{
			Prompt: "Implement database connection pool with redis",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.AssignedEngine != EngineOxiLLaMa {
			t.Errorf("expected EngineOxiLLaMa, got %v", res.AssignedEngine)
		}
		if !res.LocalComputeAvailable {
			t.Errorf("expected LocalComputeAvailable true")
		}
	})
}

func TestEngineProbingLive(t *testing.T) {
	type probeCase struct {
		name          string
		target        SupportedEngine
		ollamaModels  string
		oxillamaData  string
		ollamaStatus  int
		oxStatus      int
		wantAvailable bool
	}

	scenarios := []probeCase{
		{
			name:          "validated with llama3",
			target:        EngineOllamaValidated,
			ollamaModels:  `{"models":[{"name":"llama3:8b","model":"llama3:8b"}]}`,
			ollamaStatus:  http.StatusOK,
			wantAvailable: true,
		},
		{
			name:          "validated with other model fallback",
			target:        EngineOllamaValidated,
			ollamaModels:  `{"models":[{"name":"phi4:14b","model":"phi4:14b"}]}`,
			ollamaStatus:  http.StatusOK,
			wantAvailable: true,
		},
		{
			name:          "validated with ollama error",
			target:        EngineOllamaValidated,
			ollamaStatus:  http.StatusInternalServerError,
			wantAvailable: false,
		},
		{
			name:          "huggingface unmocked",
			target:        EngineHuggingFace,
			wantAvailable: true,
		},
		{
			name:          "candle unmocked",
			target:        EngineCandle,
			wantAvailable: true,
		},
		{
			name:          "oxillama health fallback reachable",
			target:        EngineOxiLLaMa,
			oxStatus:      http.StatusOK,
			oxillamaData:  `{"data":[]}`,
			wantAvailable: true,
		},
		{
			name:          "oxillama health fallback unreachable",
			target:        EngineOxiLLaMa,
			oxStatus:      http.StatusInternalServerError,
			wantAvailable: false,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			olSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if sc.ollamaStatus != 0 && sc.ollamaStatus != http.StatusOK {
					http.Error(w, "err", sc.ollamaStatus)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if sc.ollamaModels != "" {
					_, _ = w.Write([]byte(sc.ollamaModels))
				} else {
					_, _ = w.Write([]byte(`{"models":[]}`))
				}
			}))
			defer olSrv.Close()

			oxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if sc.oxStatus != 0 && sc.oxStatus != http.StatusOK {
					http.Error(w, "err", sc.oxStatus)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if sc.oxillamaData != "" {
					_, _ = w.Write([]byte(sc.oxillamaData))
				} else {
					_, _ = w.Write([]byte(`{"data":[]}`))
				}
			}))
			defer oxSrv.Close()

			t.Setenv("DENDRITE_OLLAMA_ENDPOINT", olSrv.URL)
			t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", oxSrv.URL)

			eng := NewEngine(nil)
			res, err := eng.Triage(context.Background(), TriageRequest{
				Prompt:       "Check syntax and format code",
				TargetEngine: sc.target,
			})
			if err != nil {
				t.Fatalf("probe triage failed: %v", err)
			}
			if res.LocalComputeAvailable != sc.wantAvailable {
				t.Errorf("got LocalComputeAvailable %v, want %v", res.LocalComputeAvailable, sc.wantAvailable)
			}
		})
	}
}

func TestDeclarativeOrchestratorExecution(t *testing.T) {
	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"local-qwen-coder","model":"local-qwen-coder"}]}`))
			return
		}
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"oxillama-rust-qwen7b"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer liveServer.Close()
	t.Setenv("DENDRITE_OLLAMA_ENDPOINT", liveServer.URL)
	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", liveServer.URL)

	eng := NewEngine(nil)

	workerCases := []struct {
		name       string
		engine     SupportedEngine
		wantWorker string
	}{
		{"validated worker", EngineOllamaValidated, "qwen2.5-coder:14b"},
		{"huggingface worker", EngineHuggingFace, "deepseek-ai/DeepSeek-Coder-V2-Lite-Instruct"},
		{"oxillama worker", EngineOxiLLaMa, "oxillama-rust-qwen7b"},
		{"candle worker", EngineCandle, "candle-quant-llama"},
		{"default worker", EngineOllama, "local-qwen-coder"},
	}

	for _, wc := range workerCases {
		t.Run(wc.name, func(t *testing.T) {
			res, err := eng.Triage(context.Background(), TriageRequest{
				Prompt:                        "Design a message queue consumer",
				TargetEngine:                  wc.engine,
				EnableDeclarativeOrchestrator: true,
				ExecuteLocal:                  true,
			})
			if err != nil {
				t.Fatalf("declarative triage failed: %v", err)
			}
			if !res.IsDeclarativeOrchestrator {
				t.Errorf("expected IsDeclarativeOrchestrator true")
			}
			if res.WorkerModel != wc.wantWorker {
				t.Errorf("got worker model %q, want %q", res.WorkerModel, wc.wantWorker)
			}
			if !strings.Contains(res.LocalWorkerExecutionResult, "Executed") {
				t.Errorf("expected LocalWorkerExecutionResult to record run, got %s", res.LocalWorkerExecutionResult)
			}
		})
	}
}

func TestTriageRAGContextRetrieval(t *testing.T) {
	eng := NewEngine(nil)

	t.Run("rag enabled with heading formatting", func(t *testing.T) {
		res, err := eng.Triage(context.Background(), TriageRequest{
			Prompt:       "Explain architecture context",
			EnableRAG:    true,
			WorkspaceDir: ".",
			RAGTopK:      3,
		})
		if err != nil {
			t.Fatalf("rag triage failed: %v", err)
		}
		if !strings.Contains(res.EnhancedPrompt, "=== RETRIEVED ARCHITECTURE & CODE CONTEXT (AST-Guided RAG) ===") {
			t.Errorf("expected enhanced prompt to contain RAG header")
		}
		if len(res.RetrievedRAGChunks) == 0 {
			t.Errorf("expected chunks to be retrieved")
		}
	})

	t.Run("rag with default topK and default workspace", func(t *testing.T) {
		res, err := eng.Triage(context.Background(), TriageRequest{
			Prompt:    "Inspect codebase symbols",
			EnableRAG: true,
			RAGTopK:   -1,
		})
		if err != nil {
			t.Fatalf("rag default triage failed: %v", err)
		}
		if res.RAGTokens <= 0 && len(res.RetrievedRAGChunks) > 0 {
			t.Errorf("expected positive rag tokens")
		}
	})

	t.Run("rag context error propagation", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := eng.Triage(canceledCtx, TriageRequest{
			Prompt:       "Inspect symbols",
			EnableRAG:    true,
			WorkspaceDir: ".",
		})
		if err == nil {
			t.Errorf("expected error from canceled context during RAG retrieval")
		}
	})
}

func TestContextCompactionEdges(t *testing.T) {
	eng := NewEngine(nil)

	t.Run("file content with only comment lines", func(t *testing.T) {
		content := "// only comment 1\n// only comment 2"
		res, err := eng.Triage(context.Background(), TriageRequest{
			Prompt:      "Review this file",
			FileContent: content,
		})
		if err != nil {
			t.Fatalf("triage failed: %v", err)
		}
		if !strings.Contains(res.EnhancedPrompt, "=== SOURCE CONTEXT (Compacted for Token Efficiency) ===") {
			t.Errorf("expected source context section in enhanced prompt")
		}
	})

	t.Run("invalid go syntax file content", func(t *testing.T) {
		badGo := "package invalid\nfunc Broken( { return"
		res, err := eng.Triage(context.Background(), TriageRequest{
			Prompt:      "Fix syntax error in this file",
			FileContent: badGo,
		})
		if err != nil {
			t.Fatalf("triage failed: %v", err)
		}
		if res.OriginalTokens <= 0 {
			t.Errorf("expected positive original tokens")
		}
	})

	t.Run("empty prompt and content minimum tokens", func(t *testing.T) {
		res, err := eng.Triage(context.Background(), TriageRequest{
			Prompt: "",
		})
		if err != nil {
			t.Fatalf("triage failed: %v", err)
		}
		if res.OriginalTokens != 1 {
			t.Errorf("expected original tokens clamped to 1, got %d", res.OriginalTokens)
		}
	})
}

func TestMatchSkillsCustom(t *testing.T) {
	custom := []SkillMatcher{
		{
			Name:      "custom-lint",
			Keywords:  []string{"customlint"},
			Guideline: "Adhere to custom lint rules.",
		},
	}

	t.Run("matching keyword", func(t *testing.T) {
		matched := MatchSkills("Run customlint now", custom)
		if len(matched) != 1 || matched[0].Name != "custom-lint" {
			t.Errorf("expected 1 matched skill, got %+v", matched)
		}
	})

	t.Run("unmatched keyword", func(t *testing.T) {
		matched := MatchSkills("Unrelated prompt text", custom)
		if len(matched) != 0 {
			t.Errorf("expected 0 matched skills, got %+v", matched)
		}
	})
}

func TestEngineConfigEdges(t *testing.T) {
	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"ox-alpha"},{"id":"ox-beta"}]}`))
			return
		}
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"ol-alpha"},{"name":"ol-beta"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer liveServer.Close()

	t.Run("preconfigured oxillama engine and endpoint", func(t *testing.T) {
		eng := NewEngine(nil)
		eng.OxiLLaMaEndpoint = liveServer.URL
		eng.OllamaEndpoint = liveServer.URL
		eng.OllamaClient = nil
		eng.OxiLLaMaEngine = inference.NewOxiLLaMaEngine(inference.EngineConfig{
			ID:       "ox-custom",
			Type:     inference.EngineOxiLLaMa,
			Endpoint: liveServer.URL,
		}, nil)

		res, err := eng.Triage(context.Background(), TriageRequest{
			Prompt: "Inspect code quality",
		})
		if err != nil {
			t.Fatalf("triage failed: %v", err)
		}
		if res == nil {
			t.Fatal("expected non-nil triage result")
		}
	})

	t.Run("engine with nil clients and empty endpoints", func(t *testing.T) {
		t.Setenv("DENDRITE_OLLAMA_ENDPOINT", "")
		t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", "")

		eng := &Engine{
			LocalModelName:    "local-qwen-coder",
			HybridModelName:   "copilot-sonnet",
			CloudModelName:    "gemini-2.5-pro",
			CostPerCloudToken: 0.000003,
		}

		res, err := eng.Triage(context.Background(), TriageRequest{
			Prompt: "Format this block",
		})
		if err != nil {
			t.Fatalf("triage failed: %v", err)
		}
		// The engine falls back to the default OxiLLaMa address (127.0.0.1:8080), so whether a local model answers
		// depends on the machine: the branch is what this case covers, the outcome only has to be a valid tier.
		switch res.Tier {
		case TierLocal, TierHybrid, TierCloud:
		default:
			t.Errorf("unexpected tier %v", res.Tier)
		}
	})
}

func TestDeclarativePlanSynthesisDirect(t *testing.T) {
	eng := NewEngine(nil)

	chunks := []rag.Chunk{
		{FilePath: "pkg/agent/triage/triage.go", LineStart: 1, LineEnd: 10, Heading: "Overview", Content: "package triage"},
		{FilePath: "pkg/agent/triage/types.go", LineStart: 1, LineEnd: 5, Content: "package triage"},
	}

	t.Run("with file skills and chunks", func(t *testing.T) {
		plan := eng.SynthesizeDeclarativePlan("Refactor database retry logic", "triage.go", []string{"ticket-lifecycle-developer"}, chunks)
		if plan == nil {
			t.Fatal("expected non-nil declarative plan")
		}
		if len(plan.Instructions) == 0 {
			t.Errorf("expected plan instructions to be generated")
		}
		if plan.OutputTokenSavingsPct <= 0 {
			t.Errorf("expected positive output token savings")
		}
	})

	t.Run("without file skills and chunks", func(t *testing.T) {
		plan := eng.SynthesizeDeclarativePlan("Routine code cleanup", "", nil, nil)
		if plan == nil {
			t.Fatal("expected non-nil declarative plan")
		}
		if plan.Goal == "" {
			t.Errorf("expected non-empty plan goal")
		}
	})
}
