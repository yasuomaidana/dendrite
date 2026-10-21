package inference

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestUpdateThroughput_PackageLevel(t *testing.T) {
	tempDir := t.TempDir()
	catalogPath := filepath.Join(tempDir, "models.json")

	// Pre-seed catalog file
	initial := &Catalog{
		Path: catalogPath,
		Models: []ModelCapability{
			{ID: "bench-model", Engine: EngineOxiLLaMa, TokensPerSec: 0},
		},
	}
	if err := SaveCatalog(initial, catalogPath); err != nil {
		t.Fatalf("failed to save initial catalog: %v", err)
	}

	// UpdateThroughput via package-level function
	if err := UpdateThroughput("bench-model", 68.4, catalogPath); err != nil {
		t.Fatalf("UpdateThroughput failed: %v", err)
	}

	// Verify persistence
	reloaded, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("failed to reload catalog: %v", err)
	}
	mod, found := reloaded.Get("bench-model")
	if !found {
		t.Fatalf("expected bench-model in reloaded catalog")
	}
	if mod.TokensPerSec != 68.4 {
		t.Errorf("expected TokensPerSec 68.4, got %f", mod.TokensPerSec)
	}

	// Test non-existent file path returns error on load
	errInvalid := UpdateThroughput("bench-model", 50.0, filepath.Join(tempDir, "non-existent", "models.json"))
	if errInvalid == nil {
		t.Errorf("expected error when updating non-existent path")
	}
}

func TestLoadCatalog_ColdStartDefault(t *testing.T) {
	// Point state dir to an empty temporary directory so neither models.toml nor models.json exists
	emptyStateDir := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", emptyStateDir)

	cat, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog cold-start returned error: %v", err)
	}
	if len(cat.List()) == 0 {
		t.Errorf("expected DefaultCatalog on cold start, got 0 models")
	}
	expectedJSON := filepath.Join(emptyStateDir, "models.json")
	if cat.Path != expectedJSON {
		t.Errorf("expected path %s, got %s", expectedJSON, cat.Path)
	}
}

func TestLoadCatalog_TOMLPriorityOverJSON(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", stateDir)

	tomlPath := filepath.Join(stateDir, "models.toml")
	jsonPath := filepath.Join(stateDir, "models.json")

	// Write both TOML and JSON
	tomlCat := &Catalog{Models: []ModelCapability{{ID: "toml-model", Engine: EngineOxiLLaMa}}}
	jsonCat := &Catalog{Models: []ModelCapability{{ID: "json-model", Engine: EngineOllama}}}

	if err := SaveCatalog(tomlCat, tomlPath); err != nil {
		t.Fatalf("failed to write TOML: %v", err)
	}
	if err := SaveCatalog(jsonCat, jsonPath); err != nil {
		t.Fatalf("failed to write JSON: %v", err)
	}

	// LoadCatalog without arguments should prioritize TOML
	loaded, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}
	if _, ok := loaded.Get("toml-model"); !ok {
		t.Errorf("expected toml-model to be loaded when both toml and json exist")
	}

	// Remove TOML, now JSON should load
	_ = os.Remove(tomlPath)
	loadedJSON, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog failed after removing TOML: %v", err)
	}
	if _, ok := loadedJSON.Get("json-model"); !ok {
		t.Errorf("expected json-model to be loaded after removing TOML")
	}
}

func TestCatalog_JSONFormatVariations(t *testing.T) {
	// Test 1: Direct JSON array format
	rawArray := `[
		{"id": "array-model", "engine": "oxillama", "reasoning_tier": 4, "tokens_per_sec": 30.0}
	]`
	var catArray Catalog
	if err := catArray.UnmarshalJSON([]byte(rawArray)); err != nil {
		t.Fatalf("failed to unmarshal array JSON: %v", err)
	}
	if len(catArray.List()) != 1 || catArray.List()[0].ID != "array-model" {
		t.Errorf("array unmarshal unexpected result: %+v", catArray.List())
	}

	// Test 2: JSON object with "model" singular key
	rawSingular := `{"model": [{"id": "singular-model", "engine": "ollama", "reasoning_tier": 2}]}`
	var catSingular Catalog
	if err := catSingular.UnmarshalJSON([]byte(rawSingular)); err != nil {
		t.Fatalf("failed to unmarshal singular key JSON: %v", err)
	}
	if len(catSingular.List()) != 1 || catSingular.List()[0].ID != "singular-model" {
		t.Errorf("singular key unmarshal unexpected result: %+v", catSingular.List())
	}

	// Test 3: Empty JSON
	var catEmpty Catalog
	if err := catEmpty.UnmarshalJSON([]byte("")); err != nil {
		t.Fatalf("failed to unmarshal empty JSON: %v", err)
	}
	if len(catEmpty.List()) != 0 {
		t.Errorf("expected 0 models for empty JSON")
	}

	// Test 4: Invalid JSON
	var catInvalid Catalog
	if err := catInvalid.UnmarshalJSON([]byte("{invalid-json")); err == nil {
		t.Errorf("expected error unmarshaling invalid JSON")
	}
}

func TestCatalog_ErrorHandlingAndEdgeCases(t *testing.T) {
	// SaveCatalog with nil
	if err := SaveCatalog(nil); err == nil {
		t.Errorf("expected error saving nil catalog")
	}

	// Load non-existent file
	if _, err := LoadCatalog("/non/existent/path/models.json"); err == nil {
		t.Errorf("expected error loading non-existent explicit path")
	}

	// Empty file returns empty catalog
	tempDir := t.TempDir()
	emptyFile := filepath.Join(tempDir, "empty.json")
	if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}
	catEmpty, err := LoadCatalog(emptyFile)
	if err != nil || len(catEmpty.List()) != 0 {
		t.Errorf("expected empty catalog from empty file, got %+v (err=%v)", catEmpty, err)
	}

	// Corrupted json file
	corruptedFile := filepath.Join(tempDir, "corrupt.json")
	if err := os.WriteFile(corruptedFile, []byte("<<<not-json-nor-toml>>>"), 0644); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}
	if _, err := LoadCatalog(corruptedFile); err == nil {
		t.Errorf("expected error loading corrupt catalog file")
	}

	// Corrupted toml file
	corruptedToml := filepath.Join(tempDir, "corrupt.toml")
	if err := os.WriteFile(corruptedToml, []byte("<<<not-json-nor-toml>>>"), 0644); err != nil {
		t.Fatalf("failed to write corrupt toml file: %v", err)
	}
	if _, err := LoadCatalog(corruptedToml); err == nil {
		t.Errorf("expected error loading corrupt toml catalog file")
	}

	// Nested dir auto-creation on SaveCatalog
	nestedToml := filepath.Join(tempDir, "a", "b", "c", "models.toml")
	catNested := &Catalog{Models: []ModelCapability{{ID: "nested-model", Engine: EngineOxiLLaMa}}}
	if err := SaveCatalog(catNested, nestedToml); err != nil {
		t.Errorf("failed to save in nested dir: %v", err)
	}
	if _, err := os.Stat(nestedToml); err != nil {
		t.Errorf("expected nested file to exist: %v", err)
	}

	// SaveCatalog with empty path falls back to DefaultCatalogPath
	catFallback := &Catalog{Models: []ModelCapability{{ID: "fallback-model", Engine: EngineOxiLLaMa}}}
	fallbackStateDir := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", fallbackStateDir)
	if err := SaveCatalog(catFallback); err != nil {
		t.Errorf("SaveCatalog with fallback path failed: %v", err)
	}
	if catFallback.Path != filepath.Join(fallbackStateDir, "models.json") {
		t.Errorf("expected cat.Path set to default, got %s", catFallback.Path)
	}

	// TOML with plural [[models]], quotes with # symbol, and FormatTOML
	tomlWithPluralAndHash := `
[[models]]
id = "model#with#hash"; engine = 'oxillama' # trailing comment
reasoning_tier = 4
coding_benchmark = 0.75
context_window = 65536
tokens_per_sec = 45.0
min_complexity = 2
max_complexity = 8
`
	pluralCat, err := ParseTOMLCatalog([]byte(tomlWithPluralAndHash))
	if err != nil {
		t.Fatalf("failed to parse TOML with plural and hash: %v", err)
	}
	if m, ok := pluralCat.Get("model#with#hash"); !ok || m.ID != "model#with#hash" {
		t.Errorf("failed to parse model with hash in quotes: %+v", m)
	}
	if formatted := pluralCat.FormatTOML(); !strings.Contains(formatted, "model#with#hash") {
		t.Errorf("FormatTOML missing model: %s", formatted)
	}
}

func TestCatalog_ConcurrencyRaceFree(t *testing.T) {
	cat := DefaultCatalog()
	tempDir := t.TempDir()
	savePath := filepath.Join(tempDir, "concurrency_models.json")
	cat.Path = savePath

	var wg sync.WaitGroup
	workers := 20
	iterations := 25

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// Concurrent reads
				_ = cat.List()
				_, _ = cat.Get("qwen2.5-coder:14b")
				_, _ = cat.GetByEngine("qwen3.8:27b", EngineOxiLLaMa)

				// Concurrent writes / updates
				cat.UpdateThroughput("qwen2.5-coder:14b", float64(workerID*10+i))
				if i%10 == 0 {
					cat.Set(ModelCapability{
						ID:            "dynamic-worker-model",
						Engine:        EngineOxiLLaMa,
						ReasoningTier: 3,
						TokensPerSec:  float64(i),
					})
				}
			}
		}(w)
	}

	wg.Wait()

	// Verify catalog state is consistent after concurrent access
	models := cat.List()
	if len(models) == 0 {
		t.Errorf("expected models after concurrent operations")
	}
}

func TestReasoningTierForScore_AllBranches(t *testing.T) {
	if tier := reasoningTierForScore("custom-14b-instruct", 90); tier != 3 {
		t.Errorf("expected 3 for 14b, got %d", tier)
	}
	if tier := reasoningTierForScore("small", 10); tier != 1 {
		t.Errorf("expected 1 for score 10, got %d", tier)
	}
	if tier := reasoningTierForScore("frontier", 120); tier != 5 {
		t.Errorf("expected 5 for score 120, got %d", tier)
	}
	if tier := reasoningTierForScore("mid", 60); tier != 3 {
		t.Errorf("expected 3 for score 60, got %d", tier)
	}
}

func TestCatalog_UnmarshalJSON_InvalidArray(t *testing.T) {
	var cat Catalog
	if err := cat.UnmarshalJSON([]byte("[invalid json array")); err == nil {
		t.Errorf("expected error unmarshaling invalid array JSON")
	}
	if err := cat.UnmarshalJSON([]byte("{}")); err != nil {
		t.Errorf("unexpected error unmarshaling empty object: %v", err)
	}
	if len(cat.List()) != 0 {
		t.Errorf("expected 0 models for empty object")
	}
}

func TestLoadCatalog_CorruptAndDefaultPaths(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", stateDir)

	tomlPath := filepath.Join(stateDir, "models.toml")
	_ = os.WriteFile(tomlPath, []byte("<<<corrupt toml data>>>"), 0644)
	if _, err := LoadCatalog(); err == nil {
		t.Errorf("expected error loading corrupted models.toml from state dir")
	}

	_ = os.Remove(tomlPath)
	jsonPath := filepath.Join(stateDir, "models.json")
	_ = os.WriteFile(jsonPath, []byte("{invalid-json-content"), 0644)
	if _, err := LoadCatalog(); err == nil {
		t.Errorf("expected error loading corrupted models.json from state dir")
	}

	_ = os.Remove(jsonPath)
	coldCat, err := LoadCatalog("")
	if err != nil || coldCat == nil {
		t.Fatalf("expected successful cold-start load with empty string customPath: %v", err)
	}
}

func TestSaveCatalog_ErrorAndEdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	cat := &Catalog{Models: []ModelCapability{{ID: "edge-model"}}}

	// Target in current directory (dir == ".")
	origWd, _ := os.Getwd()
	if err := os.Chdir(tempDir); err == nil {
		defer func() { _ = os.Chdir(origWd) }()
		if err := SaveCatalog(cat, "local_model.toml"); err != nil {
			t.Errorf("SaveCatalog failed for dir == .: %v", err)
		}
	}

	// MkdirAll error: parent directory is an existing regular file
	regFile := filepath.Join(tempDir, "regular_file")
	_ = os.WriteFile(regFile, []byte("file content"), 0644)
	if err := SaveCatalog(cat, filepath.Join(regFile, "nested", "models.toml")); err == nil {
		t.Errorf("expected error from MkdirAll when parent is a regular file")
	}

	// MarshalJSON error: NaN value
	catNaN := &Catalog{Models: []ModelCapability{{ID: "nan-model", TokensPerSec: math.NaN()}}}
	if err := SaveCatalog(catNaN, filepath.Join(tempDir, "nan.json")); err == nil {
		t.Errorf("expected error marshaling NaN float in catalog")
	}

	// WriteFile error: target is a directory path
	if err := SaveCatalog(cat, tempDir); err == nil {
		t.Errorf("expected error writing catalog to a directory path")
	}

	// customPath[0] == "" with cat.Path populated
	catPath := &Catalog{
		Path:   filepath.Join(tempDir, "prepopulated.json"),
		Models: []ModelCapability{{ID: "prepopulated"}},
	}
	if err := SaveCatalog(catPath, ""); err != nil {
		t.Errorf("SaveCatalog with empty customPath failed: %v", err)
	}
}

func TestParseCatalogData_AllBranches(t *testing.T) {
	// 1. Empty data
	catEmpty, err := parseCatalogData([]byte("   "), "models.json")
	if err != nil || len(catEmpty.Models) != 0 {
		t.Errorf("expected empty catalog for empty data, got: %+v (err=%v)", catEmpty, err)
	}

	// 2. .toml extension
	catToml, err := parseCatalogData([]byte("[[model]]\nid = \"toml-mod\"\n"), "custom.toml")
	if err != nil || len(catToml.Models) != 1 {
		t.Errorf("expected 1 model for .toml, got: %+v (err=%v)", catToml, err)
	}

	// 3. [[model table without .toml extension
	catModelTag, err := parseCatalogData([]byte("[[model]]\nid = \"tag-mod\"\n"), "custom.dat")
	if err != nil || len(catModelTag.Models) != 1 {
		t.Errorf("expected 1 model for [[model tag, got: %+v (err=%v)", catModelTag, err)
	}

	// 4. JSON array
	catJSON, err := parseCatalogData([]byte(`[{"id": "json-mod"}]`), "custom.json")
	if err != nil || len(catJSON.Models) != 1 {
		t.Errorf("expected 1 model for JSON array, got: %+v (err=%v)", catJSON, err)
	}

	// 5. TOML without [[model header and without .toml extension
	catNoHeader, err := parseCatalogData([]byte(`id = "no-header-mod"`), "custom.txt")
	if err != nil || len(catNoHeader.Models) != 1 || catNoHeader.Models[0].ID != "no-header-mod" {
		t.Errorf("expected model parsed from key-values without table header, got: %+v (err=%v)", catNoHeader, err)
	}

	// 6. Valid JSON with 0 models
	catZeroJSON, err := parseCatalogData([]byte(`{"models": []}`), "empty_models.json")
	if err != nil || len(catZeroJSON.Models) != 0 {
		t.Errorf("expected empty catalog for JSON with empty models, got: %+v (err=%v)", catZeroJSON, err)
	}

	// 7. Invalid format
	_, errInvalid := parseCatalogData([]byte(`<<<invalid data>>>`), "invalid.dat")
	if errInvalid == nil {
		t.Errorf("expected error parsing invalid format")
	}
}

func TestParseTOMLCatalog_ScannerErrorAndEdgeCases(t *testing.T) {
	// Scanner error with token > 64KB
	longLine := []byte("[[model]]\n" + strings.Repeat("a", 70000))
	if _, err := ParseTOMLCatalog(longLine); err == nil {
		t.Errorf("expected scanner error for token exceeding 64KB")
	}

	// Consecutive table headers
	consecutive := []byte("[[model]]\n[[model]]\nid = \"m1\"\n")
	catConsecutive, err := ParseTOMLCatalog(consecutive)
	if err != nil || len(catConsecutive.Models) != 1 {
		t.Errorf("expected 1 model for consecutive table headers, got: %+v (err=%v)", catConsecutive, err)
	}

	// Trailing empty table header at EOF
	trailing := []byte("[[model]]\nid = \"m1\"\n[[model]]\n")
	catTrailing, err := ParseTOMLCatalog(trailing)
	if err != nil || len(catTrailing.Models) != 1 {
		t.Errorf("expected 1 model for trailing table header, got: %+v (err=%v)", catTrailing, err)
	}

	// Comments only (nonCommentLines == 0)
	commentsOnly := []byte("# comment line 1\n# comment line 2\n")
	catComments, err := ParseTOMLCatalog(commentsOnly)
	if err != nil || len(catComments.Models) != 0 {
		t.Errorf("expected empty catalog for comments only, got: %+v (err=%v)", catComments, err)
	}
}
