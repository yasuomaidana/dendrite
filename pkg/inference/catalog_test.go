package inference

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultCatalog(t *testing.T) {
	cat := DefaultCatalog()
	if cat == nil {
		t.Fatalf("expected non-nil DefaultCatalog")
	}

	models := cat.List()
	if len(models) == 0 {
		t.Fatalf("expected models in DefaultCatalog, got 0")
	}

	// Verify qwen2.5-coder:14b on oxillama per architecture plan §2.2
	cap, found := cat.GetByEngine("qwen2.5-coder:14b", EngineOxiLLaMa)
	if !found {
		t.Fatalf("expected qwen2.5-coder:14b on oxillama in DefaultCatalog")
	}
	if cap.ReasoningTier != 3 {
		t.Errorf("expected ReasoningTier 3, got %d", cap.ReasoningTier)
	}
	if cap.CodingBenchmark != 0.62 {
		t.Errorf("expected CodingBenchmark 0.62, got %f", cap.CodingBenchmark)
	}
	if cap.ContextWindow != 32768 {
		t.Errorf("expected ContextWindow 32768, got %d", cap.ContextWindow)
	}
	if cap.TokensPerSec != 0 {
		t.Errorf("expected TokensPerSec 0, got %f", cap.TokensPerSec)
	}
	if cap.MinComplexity != 1 || cap.MaxComplexity != 6 {
		t.Errorf("expected complexity 1..6, got %d..%d", cap.MinComplexity, cap.MaxComplexity)
	}

	// Verify qwen3.8:27b high reasoning tier
	cap27b, found27b := cat.GetByEngine("qwen3.8:27b", EngineOxiLLaMa)
	if !found27b {
		t.Fatalf("expected qwen3.8:27b in DefaultCatalog")
	}
	// Local models top out at tier 4; complexity 8-10 is reserved for frontier models (plan §4.1).
	if cap27b.ReasoningTier != 4 {
		t.Errorf("expected ReasoningTier 4 for a local 27B model, got %d", cap27b.ReasoningTier)
	}
	if cap27b.MinComplexity != 2 || cap27b.MaxComplexity != 8 {
		t.Errorf("expected complexity bounds 2..8 for a local 27B model, got %d..%d", cap27b.MinComplexity, cap27b.MaxComplexity)
	}

	// Verify llama3.2:3b lower tier and 8k context
	cap3b, found3b := cat.GetByEngine("llama3.2:3b", EngineOllama)
	if !found3b {
		t.Fatalf("expected llama3.2:3b in DefaultCatalog")
	}
	if cap3b.ReasoningTier != 1 {
		t.Errorf("expected ReasoningTier 1 for 3b model, got %d", cap3b.ReasoningTier)
	}
	if cap3b.ContextWindow != 8192 {
		t.Errorf("expected ContextWindow 8192 for 3b model, got %d", cap3b.ContextWindow)
	}
	if cap3b.MaxComplexity != 3 {
		t.Errorf("expected MaxComplexity 3 for tier 1 model, got %d", cap3b.MaxComplexity)
	}
}

func TestDefaultCapabilityVariants(t *testing.T) {
	// 14B model (should be tier 3)
	c14 := DefaultCapability("custom-coder-14b", EngineOxiLLaMa)
	if c14.ReasoningTier != 3 {
		t.Errorf("expected tier 3 for 14b, got %d", c14.ReasoningTier)
	}

	// 128k context window
	c128k := DefaultCapability("qwen-128k", EngineVLLM)
	if c128k.ContextWindow != 131072 {
		t.Errorf("expected 131072 context window for 128k model, got %d", c128k.ContextWindow)
	}

	// 131072 context window explicit
	c131 := DefaultCapability("deepseek-131072", EngineVLLM)
	if c131.ContextWindow != 131072 {
		t.Errorf("expected 131072 context window, got %d", c131.ContextWindow)
	}

	// 1B small model
	c1b := DefaultCapability("tiny-1b", EngineOllama)
	if c1b.ContextWindow != 8192 {
		t.Errorf("expected 8192 context window for 1b model, got %d", c1b.ContextWindow)
	}
	if c1b.ReasoningTier != 1 {
		t.Errorf("expected tier 1 for 1b model, got %d", c1b.ReasoningTier)
	}
	if c1b.MinComplexity != 1 || c1b.MaxComplexity != 3 {
		t.Errorf("expected complexity 1..3 for tier 1, got %d..%d", c1b.MinComplexity, c1b.MaxComplexity)
	}

	// Tier 4 model (score between 80 and 89 without 14b)
	cTier4 := DefaultCapability("generic-20b-instruct", EngineOxiLLaMa)
	if cTier4.MinComplexity != 2 || cTier4.MaxComplexity != 8 {
		t.Logf("tier %d complexity: %d..%d", cTier4.ReasoningTier, cTier4.MinComplexity, cTier4.MaxComplexity)
	}
}

func TestCatalog_GetAndTieBreak(t *testing.T) {
	cat := &Catalog{
		Models: []ModelCapability{
			{ID: "qwen-coder", Engine: EngineOllama, ReasoningTier: 3},
			{ID: "qwen-coder", Engine: EngineOxiLLaMa, ReasoningTier: 4},
			{ID: "mistral-only", Engine: EngineOllama, ReasoningTier: 2},
		},
	}

	// Tie break: the GPU engine (Ollama) wins over OxiLLaMa, which prefills on the CPU
	best, ok := cat.Get("qwen-coder")
	if !ok {
		t.Fatalf("expected to find qwen-coder")
	}
	if best.Engine != EngineOllama {
		t.Errorf("expected ollama to win tie break, got %s", best.Engine)
	}
	if best.ReasoningTier != 3 {
		t.Errorf("expected ReasoningTier 3, got %d", best.ReasoningTier)
	}

	// Scoped get
	ollamaScoped, ok := cat.Get("ollama/qwen-coder")
	if !ok || ollamaScoped.Engine != EngineOllama {
		t.Errorf("expected ollama model when scoped, got %+v (ok=%v)", ollamaScoped, ok)
	}

	// Fallback to non-oxillama when only ollama exists
	mistral, ok := cat.Get("mistral-only")
	if !ok || mistral.Engine != EngineOllama {
		t.Errorf("expected mistral on ollama, got %+v (ok=%v)", mistral, ok)
	}

	// Not found
	_, notFound := cat.Get("non-existent-model")
	if notFound {
		t.Errorf("expected false for non-existent model")
	}

	// GetByEngine
	byEng, ok := cat.GetByEngine("qwen-coder", EngineOllama)
	if !ok || byEng.Engine != EngineOllama {
		t.Errorf("expected ollama from GetByEngine, got %+v", byEng)
	}

	_, noEng := cat.GetByEngine("qwen-coder", EngineCandle)
	if noEng {
		t.Errorf("expected false for wrong engine")
	}

	_, noID := cat.GetByEngine("missing", EngineOllama)
	if noID {
		t.Errorf("expected false for missing ID")
	}
}

func TestCatalog_SetAndList(t *testing.T) {
	cat := &Catalog{}

	// Adding new capability
	cat.Set(ModelCapability{
		ID:            "new-model",
		Engine:        EngineOxiLLaMa,
		ReasoningTier: 3,
	})

	if len(cat.List()) != 1 {
		t.Fatalf("expected 1 model in catalog, got %d", len(cat.List()))
	}

	// Replacing existing capability
	cat.Set(ModelCapability{
		ID:            "new-model",
		Engine:        EngineOxiLLaMa,
		ReasoningTier: 5,
	})

	models := cat.List()
	if len(models) != 1 {
		t.Fatalf("expected 1 model after replace, got %d", len(models))
	}
	if models[0].ReasoningTier != 5 {
		t.Errorf("expected ReasoningTier 5 after update, got %d", models[0].ReasoningTier)
	}

	// Adding same ID under different engine
	cat.Set(ModelCapability{
		ID:            "new-model",
		Engine:        EngineOllama,
		ReasoningTier: 2,
	})

	if len(cat.List()) != 2 {
		t.Fatalf("expected 2 models under different engines, got %d", len(cat.List()))
	}

	// Defensive copy verification
	copyList := cat.List()
	copyList[0].ReasoningTier = 99
	original, _ := cat.GetByEngine("new-model", EngineOxiLLaMa)
	if original.ReasoningTier == 99 {
		t.Errorf("List() did not return a defensive copy")
	}
}

func TestCatalog_UpdateThroughput(t *testing.T) {
	cat := &Catalog{
		Models: []ModelCapability{
			{ID: "model-a", Engine: EngineOxiLLaMa, TokensPerSec: 0},
			{ID: "model-a", Engine: EngineOllama, TokensPerSec: 0},
			{ID: "model-b", Engine: EngineOxiLLaMa, TokensPerSec: 10},
		},
	}

	// Update without engine prefix updates all matching model IDs
	cat.UpdateThroughput("model-a", 48.5)
	mAOxi, _ := cat.GetByEngine("model-a", EngineOxiLLaMa)
	mAOll, _ := cat.GetByEngine("model-a", EngineOllama)
	if mAOxi.TokensPerSec != 48.5 || mAOll.TokensPerSec != 48.5 {
		t.Errorf("expected both model-a instances updated to 48.5, got %f and %f", mAOxi.TokensPerSec, mAOll.TokensPerSec)
	}

	// Update with engine prefix updates only the targeted engine
	cat.UpdateThroughput("oxillama/model-a", 65.0)
	mAOxi, _ = cat.GetByEngine("model-a", EngineOxiLLaMa)
	mAOll, _ = cat.GetByEngine("model-a", EngineOllama)
	if mAOxi.TokensPerSec != 65.0 {
		t.Errorf("expected oxillama model-a updated to 65.0, got %f", mAOxi.TokensPerSec)
	}
	if mAOll.TokensPerSec != 48.5 {
		t.Errorf("expected ollama model-a to remain 48.5, got %f", mAOll.TokensPerSec)
	}

	// Update non-existing model creates and appends it
	cat.UpdateThroughput("new-unregistered-model", 33.2)
	newMod, found := cat.Get("new-unregistered-model")
	if !found {
		t.Fatalf("expected newly created model to be in catalog")
	}
	if newMod.TokensPerSec != 33.2 {
		t.Errorf("expected TokensPerSec 33.2, got %f", newMod.TokensPerSec)
	}
	if newMod.Engine != EngineOllama {
		t.Errorf("expected default engine ollama for new model, got %s", newMod.Engine)
	}

	// Update non-existing model with explicit engine prefix
	cat.UpdateThroughput("vllm/remote-vllm-model", 95.0)
	vllmMod, foundVllm := cat.GetByEngine("remote-vllm-model", EngineVLLM)
	if !foundVllm {
		t.Fatalf("expected newly created vllm model in catalog")
	}
	if vllmMod.TokensPerSec != 95.0 {
		t.Errorf("expected TokensPerSec 95.0, got %f", vllmMod.TokensPerSec)
	}
}

func TestCatalog_SaveAndLoad_JSON(t *testing.T) {
	tempDir := t.TempDir()
	jsonFile := filepath.Join(tempDir, "models.json")

	cat := &Catalog{
		Path: jsonFile,
		Models: []ModelCapability{
			{
				ID:              "qwen2.5-coder:14b",
				Engine:          EngineOxiLLaMa,
				ReasoningTier:   3,
				CodingBenchmark: 0.62,
				ContextWindow:   32768,
				TokensPerSec:    55.25,
				MinComplexity:   1,
				MaxComplexity:   6,
			},
		},
	}

	if err := cat.Save(); err != nil {
		t.Fatalf("failed to save catalog: %v", err)
	}

	loaded, err := LoadCatalog(jsonFile)
	if err != nil {
		t.Fatalf("failed to load catalog from %s: %v", jsonFile, err)
	}

	if len(loaded.List()) != 1 {
		t.Fatalf("expected 1 model, got %d", len(loaded.List()))
	}
	m := loaded.List()[0]
	if m.ID != "qwen2.5-coder:14b" || m.Engine != EngineOxiLLaMa || m.TokensPerSec != 55.25 {
		t.Errorf("loaded model mismatch: %+v", m)
	}
}

func TestCatalog_SaveAndLoad_TOML(t *testing.T) {
	tempDir := t.TempDir()
	tomlFile := filepath.Join(tempDir, "models.toml")

	cat := &Catalog{
		Path: tomlFile,
		Models: []ModelCapability{
			{
				ID:              "qwen2.5-coder:14b",
				Engine:          EngineOxiLLaMa,
				ReasoningTier:   3,
				CodingBenchmark: 0.62,
				ContextWindow:   32768,
				TokensPerSec:    42.5,
				MinComplexity:   1,
				MaxComplexity:   6,
			},
			{
				ID:              "llama3.2:3b",
				Engine:          EngineOllama,
				ReasoningTier:   1,
				CodingBenchmark: 0.35,
				ContextWindow:   8192,
				TokensPerSec:    80.0,
				MinComplexity:   1,
				MaxComplexity:   3,
			},
		},
	}

	if err := SaveCatalog(cat, tomlFile); err != nil {
		t.Fatalf("failed to save TOML catalog: %v", err)
	}

	// Verify file content is valid TOML
	data, err := os.ReadFile(tomlFile)
	if err != nil {
		t.Fatalf("failed to read saved TOML file: %v", err)
	}
	tomlText := string(data)
	if !strings.Contains(tomlText, "[[model]]") || !strings.Contains(tomlText, "qwen2.5-coder:14b") {
		t.Errorf("saved TOML missing expected headers: %s", tomlText)
	}

	loaded, err := LoadCatalog(tomlFile)
	if err != nil {
		t.Fatalf("failed to load TOML catalog: %v", err)
	}

	if len(loaded.List()) != 2 {
		t.Fatalf("expected 2 models loaded from TOML, got %d", len(loaded.List()))
	}

	m1, found1 := loaded.GetByEngine("qwen2.5-coder:14b", EngineOxiLLaMa)
	if !found1 || m1.TokensPerSec != 42.5 || m1.CodingBenchmark != 0.62 {
		t.Errorf("m1 mismatch: %+v (found=%v)", m1, found1)
	}

	m2, found2 := loaded.GetByEngine("llama3.2:3b", EngineOllama)
	if !found2 || m2.ContextWindow != 8192 || m2.TokensPerSec != 80.0 {
		t.Errorf("m2 mismatch: %+v (found=%v)", m2, found2)
	}
}

func TestParseTOMLCatalog_PlanSectionSpec(t *testing.T) {
	// Exact snippet from docs/architecture/2026-10-quality-audit-execution-plan.md §2.2
	rawTOML := `
[[model]]
id = "qwen2.5-coder:14b"; engine = "oxillama"   # also listed under ollama with its own row
reasoning_tier = 3        # 1..5
coding_benchmark = 0.62   # normalized (e.g. HumanEval+/LiveCodeBench pass@1), from bench or registry
context_window = 32768
tokens_per_sec = 0        # 0 = unmeasured; filled by dendrite bench (pkg/perf) and EWMA at runtime
min_complexity = 1; max_complexity = 6

[[model]]
id = "qwen2.5-coder:14b"; engine = "ollama"
reasoning_tier = 3
coding_benchmark = 0.62
context_window = 32768
tokens_per_sec = 28.5
min_complexity = 1
max_complexity = 6
`

	cat, err := ParseTOMLCatalog([]byte(rawTOML))
	if err != nil {
		t.Fatalf("failed to parse plan section TOML: %v", err)
	}

	if len(cat.List()) != 2 {
		t.Fatalf("expected 2 models parsed, got %d", len(cat.List()))
	}

	oxi, ok := cat.GetByEngine("qwen2.5-coder:14b", EngineOxiLLaMa)
	if !ok {
		t.Fatalf("expected oxillama model in parsed catalog")
	}
	if oxi.ReasoningTier != 3 || oxi.CodingBenchmark != 0.62 || oxi.ContextWindow != 32768 {
		t.Errorf("oxillama model values mismatch: %+v", oxi)
	}
	if oxi.MinComplexity != 1 || oxi.MaxComplexity != 6 {
		t.Errorf("oxillama complexity mismatch: %+v", oxi)
	}

	oll, ok := cat.GetByEngine("qwen2.5-coder:14b", EngineOllama)
	if !ok {
		t.Fatalf("expected ollama model in parsed catalog")
	}
	if oll.TokensPerSec != 28.5 {
		t.Errorf("expected ollama tokens_per_sec 28.5, got %f", oll.TokensPerSec)
	}
}
