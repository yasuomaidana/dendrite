package inference

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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
