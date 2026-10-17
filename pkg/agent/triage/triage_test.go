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

func TestMain(m *testing.M) {
	tsOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags" {
			w.Write([]byte(`{"models":[{"name":"local-qwen-coder","model":"local-qwen-coder"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))

	tsOxillama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			w.Write([]byte(`{"data":[{"id":"oxillama-rust-qwen7b"}]}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))

	os.Setenv("DENDRITE_OXILLAMA_ENDPOINT", tsOxillama.URL)
	os.Setenv("DENDRITE_OLLAMA_ENDPOINT", tsOllama.URL)

	code := m.Run()
	tsOllama.Close()
	tsOxillama.Close()
	os.Exit(code)
}

func TestClassifyTaskComplexity(t *testing.T) {
	tests := []struct {
		name         string
		prompt       string
		expectedTier ExecutionTier
	}{
		{
			name:         "Chore: Lint error explanation",
			prompt:       "Explain this linter warning and fix the formatting",
			expectedTier: TierLocal,
		},
		{
			name:         "Chore: Unit test vector generation",
			prompt:       "Generate unit test vectors and boilerplate mocks for this struct",
			expectedTier: TierLocal,
		},
		{
			name:         "Chore: Docstring authoring",
			prompt:       "Write godoc docstring comments for all exported functions",
			expectedTier: TierLocal,
		},
		{
			name:         "Macro: Strategic Architecture",
			prompt:       "Design the macro architecture for distributed consensus across multi-region clusters",
			expectedTier: TierCloud,
		},
		{
			name:         "Macro: Threat Model & Formal Proof",
			prompt:       "Author the threat model and formal verification proof for our cryptographic protocol",
			expectedTier: TierCloud,
		},
		{
			name:         "Hybrid: Feature Development",
			prompt:       "Implement an HTTP health check handler that checks Redis connection pool status",
			expectedTier: TierHybrid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, reason := ClassifyTaskComplexity(tt.prompt)
			if tier != tt.expectedTier {
				t.Errorf("expected tier %v, got %v (reason: %s)", tt.expectedTier, tier, reason)
			}
		})
	}
}

func TestSkillMatching(t *testing.T) {
	prompt := "Check ticket DEN-050 in Plane and evaluate cyclomatic complexity before refactoring"
	matched := MatchSkills(prompt, nil)

	foundTicket := false
	foundComplexity := false
	for _, m := range matched {
		if m.Name == "ticket-lifecycle-developer" {
			foundTicket = true
		}
		if m.Name == "pattern-complexity-auditor" {
			foundComplexity = true
		}
	}

	if !foundTicket {
		t.Errorf("expected skill 'ticket-lifecycle-developer' to be matched")
	}
	if !foundComplexity {
		t.Errorf("expected skill 'pattern-complexity-auditor' to be matched")
	}
}

func TestTriageChoreWithLocalCompute(t *testing.T) {
	engine := NewEngine(nil)
	computeTrue := true
	engine.MockComputeAvailable = &computeTrue

	req := TriageRequest{
		Prompt:       "Fix formatting and add docstrings for this struct",
		ExecuteLocal: true,
	}

	res, err := engine.Triage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected triage error: %v", err)
	}

	if res.Tier != TierLocal {
		t.Errorf("expected tier 'local', got %s", res.Tier)
	}
	if res.AssignedModel != "local-qwen-coder" {
		t.Errorf("expected local model 'local-qwen-coder', got %s", res.AssignedModel)
	}
	if res.EstimatedCostUSD != 0.0 {
		t.Errorf("expected $0.00 cost for local chore, got %f", res.EstimatedCostUSD)
	}
	if res.TokenSavingsPct != 100.0 {
		t.Errorf("expected 100%% token savings for local chore, got %f", res.TokenSavingsPct)
	}
	if !strings.Contains(res.LocalExecutionResult, "zero cloud tokens") {
		t.Errorf("expected local execution message, got %s", res.LocalExecutionResult)
	}
}

func TestTriageChoreFallbackWhenLocalOffline(t *testing.T) {
	engine := NewEngine(nil)
	computeFalse := false
	engine.MockComputeAvailable = &computeFalse

	req := TriageRequest{
		Prompt: "Fix formatting and add docstrings for this struct",
	}

	res, err := engine.Triage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected triage error: %v", err)
	}

	// Should escalate from local to hybrid because local is offline
	if res.Tier != TierHybrid {
		t.Errorf("expected tier 'hybrid' on offline local engine, got %s", res.Tier)
	}
	if res.LocalComputeAvailable {
		t.Errorf("expected LocalComputeAvailable to be false")
	}
	if !strings.Contains(res.Reason, "offline") {
		t.Errorf("expected reason to mention offline local compute, got %s", res.Reason)
	}
}

func TestTriageForceTierAndCloudMacro(t *testing.T) {
	engine := NewEngine(nil)
	computeTrue := true
	engine.MockComputeAvailable = &computeTrue

	// 1. Cloud macro task
	macroReq := TriageRequest{
		Prompt: "Design the macro architecture for distributed consensus across nodes",
	}
	macroRes, err := engine.Triage(context.Background(), macroReq)
	if err != nil {
		t.Fatalf("unexpected macro triage error: %v", err)
	}
	if macroRes.Tier != TierCloud {
		t.Errorf("expected tier 'cloud', got %s", macroRes.Tier)
	}
	if macroRes.TokenSavingsPct != 0.0 {
		t.Errorf("expected 0%% token savings on pure cloud macro, got %f", macroRes.TokenSavingsPct)
	}

	// 2. Explicit force tier override
	overrideReq := TriageRequest{
		Prompt:    "Design macro architecture",
		ForceTier: TierLocal,
	}
	overrideRes, err := engine.Triage(context.Background(), overrideReq)
	if err != nil {
		t.Fatalf("unexpected override triage error: %v", err)
	}
	if overrideRes.Tier != TierLocal {
		t.Errorf("expected overridden tier 'local', got %s", overrideRes.Tier)
	}
}

func TestTriageNonGoFileCompaction(t *testing.T) {
	engine := NewEngine(nil)
	computeTrue := true
	engine.MockComputeAvailable = &computeTrue

	rawText := `// Line 1 comments
line 2 text here is somewhat long to ensure tokens are counted
// Line 3 comments
line 4 text here is also quite long and informative
// Line 5 comments`

	req := TriageRequest{
		Prompt:      "Explain these lines",
		FileContent: rawText,
	}

	res, err := engine.Triage(context.Background(), req)
	if err != nil {
		t.Fatalf("triage failed: %v", err)
	}

	if res.CompactedTokens >= res.OriginalTokens {
		t.Errorf("expected non-Go comment stripping to reduce tokens: compacted=%d, orig=%d", res.CompactedTokens, res.OriginalTokens)
	}
}

func TestTriageHybridContextCompactionAndSkillEnrichment(t *testing.T) {
	engine := NewEngine(nil)
	computeTrue := true
	engine.MockComputeAvailable = &computeTrue

	sampleGoCode := `package example

import "fmt"

type Server struct {
	Port int
}

func (s *Server) Start() error {
	fmt.Println("Starting server on port", s.Port)
	// Some very long implementation lines
	for i := 0; i < 100; i++ {
		fmt.Printf("step %d\n", i)
	}
	return nil
}

func (s *Server) Stop() {
	fmt.Println("Stopping")
}
`

	req := TriageRequest{
		Prompt:      "Implement graceful shutdown for this server respecting ticket DEN-050",
		FileContent: sampleGoCode,
	}

	res, err := engine.Triage(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected triage error: %v", err)
	}

	if res.Tier != TierHybrid {
		t.Errorf("expected tier 'hybrid', got %s", res.Tier)
	}

	// Verify skill injection
	if len(res.SelectedSkills) == 0 {
		t.Errorf("expected selected skills, got none")
	}
	if !strings.Contains(res.EnhancedPrompt, "=== ACTIVE SKILL DIRECTIVES ===") {
		t.Errorf("expected skill directives in enhanced prompt")
	}

	// Verify token compaction
	if res.CompactedTokens >= res.OriginalTokens {
		t.Errorf("expected compacted tokens (%d) < original tokens (%d)", res.CompactedTokens, res.OriginalTokens)
	}
	if res.TokenSavingsPct <= 0 {
		t.Errorf("expected positive token savings, got %f%%", res.TokenSavingsPct)
	}
	if res.EstimatedCostUSD >= res.CostWithoutTriageUSD {
		t.Errorf("expected estimated cost (%f) < cost without triage (%f)", res.EstimatedCostUSD, res.CostWithoutTriageUSD)
	}
}

func TestTokenEfficiencyBenchmarkWorkload(t *testing.T) {
	engine := NewEngine(nil)
	computeTrue := true
	engine.MockComputeAvailable = &computeTrue

	// Representative developer workload where chores include real source code files
	// (formatting, adding docstrings, fixing syntax, generating test boilerplate for structs)
	workloads := []TriageRequest{
		{
			Prompt: "Explain this linter warning and fix formatting for this function",
			FileContent: `func ProcessData(x int) int {
				// unformatted code
				var res = x * 2; return res
			}`,
		},
		{
			Prompt: "Generate unit test boilerplate vectors for UserAuthentication struct",
			FileContent: `type UserAuthentication struct {
				UserID string
				Token  string
				Expiry int64
			}`,
		},
		{
			Prompt: "Write docstring comment for ExportedMethod and interface",
			FileContent: `type Reader interface {
				Read(p []byte) (n int, err error)
			}`,
		},
		{
			Prompt: "Fix syntax error and format this function",
			FileContent: `func Validate(s string) bool {
				return len(s) > 0
			}`,
		},
		{
			Prompt: "Add json tags and boilerplate getters to UserProfile struct",
			FileContent: `type UserProfile struct {
				ID        string
				Email     string
				FullName  string
				CreatedAt int64
			}`,
		},
		{
			Prompt: "Implement telemetry counter with ticket DEN-050",
			FileContent: `package telemetry
import "fmt"
type Gauge struct { V int }
func (g *Gauge) Record() {
	for i := 0; i < 50; i++ {
		fmt.Println(i)
	}
}`,
		},
		{
			Prompt: "Add cache TTL eviction with pattern-complexity guidelines",
			FileContent: `package cache
type Store struct { Items map[string]int }
func (s *Store) Prune() {
	for k, v := range s.Items {
		delete(s.Items, k)
		_ = v
	}
}`,
		},
		{Prompt: "Design macro architecture for distributed consensus across multi-region clusters"},
	}

	totalOriginalTokens := 0
	totalConsumedCloudTokens := 0
	totalCostWithoutTriage := 0.0
	totalCostWithTriage := 0.0

	for _, w := range workloads {
		res, err := engine.Triage(context.Background(), w)
		if err != nil {
			t.Fatalf("triage failed: %v", err)
		}
		totalOriginalTokens += res.OriginalTokens
		totalCostWithoutTriage += res.CostWithoutTriageUSD
		totalCostWithTriage += res.EstimatedCostUSD

		if res.Tier == TierLocal {
			// 0 cloud tokens consumed!
		} else {
			totalConsumedCloudTokens += res.CompactedTokens
		}
	}

	overallTokenSavingsPct := float64(totalOriginalTokens-totalConsumedCloudTokens) / float64(totalOriginalTokens) * 100.0
	overallCostSavingsPct := (totalCostWithoutTriage - totalCostWithTriage) / totalCostWithoutTriage * 100.0

	t.Logf("Total Original Tokens: %d", totalOriginalTokens)
	t.Logf("Total Consumed Cloud Tokens: %d", totalConsumedCloudTokens)
	t.Logf("Overall Token Savings: %.1f%%", overallTokenSavingsPct)
	t.Logf("Overall Cost Savings: %.1f%%", overallCostSavingsPct)

	if overallTokenSavingsPct < 60.0 {
		t.Errorf("expected overall token savings >= 60%%, got %.1f%%", overallTokenSavingsPct)
	}
	if overallCostSavingsPct < 60.0 {
		t.Errorf("expected overall cost savings >= 60%%, got %.1f%%", overallCostSavingsPct)
	}
}

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

func TestTriage_OxiLLaMaPrecedenceOverLessSmartOllamaModel(t *testing.T) {
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

	// Because Ollama only has llama3.2:3b (score 38) and OxiLLaMa has 7b (score 60),
	// smarter local model rule gives precedence to OxiLLaMa!
	if res.AssignedEngine != EngineOxiLLaMa {
		t.Errorf("expected EngineOxiLLaMa to take precedence over less-smart Ollama model, got %s", res.AssignedEngine)
	}
	if res.AssignedModel != "oxillama-rust-qwen7b" {
		t.Errorf("expected assigned model 'oxillama-rust-qwen7b', got %s", res.AssignedModel)
	}
	if !strings.Contains(res.Reason, "OxiLLaMa") {
		t.Errorf("expected reason to document OxiLLaMa precedence, got: %s", res.Reason)
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

	if res.AssignedEngine != EngineOxiLLaMa {
		t.Errorf("expected EngineOxiLLaMa to win over Ollama, got %s", res.AssignedEngine)
	}
	if res.AssignedModel != "qwen3.8:27b" {
		t.Errorf("expected assigned model qwen3.8:27b, got %s", res.AssignedModel)
	}
	if !strings.Contains(res.Reason, "Score 96") {
		t.Errorf("expected reason to cite Score 96, got: %s", res.Reason)
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
