package inference_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"Dendrite/pkg/inference"
)

func TestZeroCostRegistry_Defaults(t *testing.T) {
	reg := inference.NewZeroCostRegistry("")

	expectedModels := []string{
		"local-qwen-coder",
		"local-qwen3-27b",
		"local-qwen3-9b",
		"oxillama-rust",
		"vllm-cluster",
		"hf-tgi-remote",
		"llamacpp-local",
	}

	for _, m := range expectedModels {
		if !reg.IsZeroCost(m) {
			t.Errorf("expected default model %s to be zero cost", m)
		}
	}

	expectedPrefixes := []string{
		"ollama/qwen2.5-coder",
		"oxillama/mistral-7b",
		"vllm/custom-model",
		"llamacpp/llama-3",
		"huggingface-tei/bge-large",
	}

	for _, p := range expectedPrefixes {
		if !reg.IsZeroCost(p) {
			t.Errorf("expected default prefix %s to be zero cost", p)
		}
	}

	commercial := []string{
		"gemini-2.5-pro",
		"gemini-2.5-flash",
		"copilot-sonnet",
		"openai/gpt-4o",
		"anthropic/claude-3-5-sonnet",
	}

	for _, c := range commercial {
		if reg.IsZeroCost(c) {
			t.Errorf("expected commercial model %s to NOT be zero cost", c)
		}
	}
}

func TestZeroCostRegistry_CaseInsensitivity(t *testing.T) {
	reg := inference.NewZeroCostRegistry("")

	if !reg.IsZeroCost("LOCAL-QWEN-CODER") {
		t.Errorf("expected case-insensitive match for LOCAL-QWEN-CODER")
	}
	if !reg.IsZeroCost("  Ollama/Qwen3.8:27b  ") {
		t.Errorf("expected trimmed case-insensitive match for Ollama/Qwen3.8:27b")
	}
}

func TestZeroCostRegistry_AddAndRemove(t *testing.T) {
	reg := inference.NewZeroCostRegistry("")

	// Initially custom model is not zero cost
	if reg.IsZeroCost("my-custom-model") {
		t.Errorf("did not expect my-custom-model to be zero cost initially")
	}

	// Add exact model
	reg.AddModel("my-custom-model")
	if !reg.IsZeroCost("my-custom-model") {
		t.Errorf("expected my-custom-model to be zero cost after AddModel")
	}

	// Remove exact model
	reg.RemoveModel("my-custom-model")
	if reg.IsZeroCost("my-custom-model") {
		t.Errorf("did not expect my-custom-model to be zero cost after RemoveModel")
	}

	// Remove a default model
	if !reg.IsZeroCost("local-qwen3-9b") {
		t.Errorf("expected local-qwen3-9b to be zero cost initially")
	}
	reg.RemoveModel("local-qwen3-9b")
	if reg.IsZeroCost("local-qwen3-9b") {
		t.Errorf("expected local-qwen3-9b to NOT be zero cost after removal")
	}

	// Add custom prefix
	if reg.IsZeroCost("my-engine/deepseek-r1") {
		t.Errorf("did not expect my-engine prefix to be zero cost initially")
	}
	reg.AddPrefix("my-engine/")
	if !reg.IsZeroCost("my-engine/deepseek-r1") {
		t.Errorf("expected my-engine/deepseek-r1 to be zero cost after AddPrefix")
	}

	// Remove custom prefix
	reg.RemovePrefix("my-engine/")
	if reg.IsZeroCost("my-engine/deepseek-r1") {
		t.Errorf("did not expect my-engine/deepseek-r1 to be zero cost after RemovePrefix")
	}
}

func TestZeroCostRegistry_AddRemoveAutoDetect(t *testing.T) {
	reg := inference.NewZeroCostRegistry("")

	// Add with trailing slash auto-detected as prefix
	ok, isPrefix := reg.Add("custom-prefix/")
	if !ok || !isPrefix {
		t.Errorf("expected custom-prefix/ to be detected as prefix")
	}
	if !reg.IsZeroCost("custom-prefix/model-x") {
		t.Errorf("expected custom-prefix/model-x to match prefix")
	}

	// Add without trailing slash auto-detected as exact model
	ok, isPrefix = reg.Add("exact-unique-model")
	if !ok || isPrefix {
		t.Errorf("expected exact-unique-model to be detected as model")
	}
	if !reg.IsZeroCost("exact-unique-model") {
		t.Errorf("expected exact-unique-model to match")
	}
	if reg.IsZeroCost("exact-unique-model-extended") {
		t.Errorf("did not expect substring match for exact model")
	}

	// Remove with auto-detect
	reg.Remove("custom-prefix/")
	if reg.IsZeroCost("custom-prefix/model-x") {
		t.Errorf("expected custom-prefix/model-x to be removed")
	}
	reg.Remove("exact-unique-model")
	if reg.IsZeroCost("exact-unique-model") {
		t.Errorf("expected exact-unique-model to be removed")
	}
}

func TestZeroCostRegistry_TestDiagnostics(t *testing.T) {
	reg := inference.NewZeroCostRegistry("")

	match, reason := reg.Test("local-qwen3-27b")
	if !match || reason == "" {
		t.Errorf("expected match for local-qwen3-27b, got match=%v reason=%s", match, reason)
	}

	match, reason = reg.Test("ollama/llama3.2")
	if !match || reason == "" {
		t.Errorf("expected match for ollama/llama3.2, got match=%v reason=%s", match, reason)
	}

	match, reason = reg.Test("gemini-2.5-pro")
	if match {
		t.Errorf("expected commercial model to fail, got reason=%s", reason)
	}
}

func TestZeroCostRegistry_EnvLoading(t *testing.T) {
	t.Setenv(inference.EnvZeroCostModels, "env-model-1,env-model-2,env-prefix/")
	t.Setenv(inference.EnvZeroCostPrefixes, "custom-prefix-1,custom-prefix-2/")

	reg := inference.NewZeroCostRegistry("")
	reg.LoadFromEnv()

	if !reg.IsZeroCost("env-model-1") {
		t.Errorf("expected env-model-1 to be zero cost")
	}
	if !reg.IsZeroCost("env-model-2") {
		t.Errorf("expected env-model-2 to be zero cost")
	}
	if !reg.IsZeroCost("env-prefix/some-model") {
		t.Errorf("expected env-prefix/some-model to be zero cost")
	}
	if !reg.IsZeroCost("custom-prefix-1/some-model") {
		t.Errorf("expected custom-prefix-1/some-model to be zero cost")
	}
	if !reg.IsZeroCost("custom-prefix-2/some-model") {
		t.Errorf("expected custom-prefix-2/some-model to be zero cost")
	}
}

func TestZeroCostRegistry_EnvFileLoadingAndSaving(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	initialContent := `# Test Environment
SOME_KEY=value
DENDRITE_ZERO_COST_MODELS=file-model-1,file-prefix/
DENDRITE_ZERO_COST_PREFIXES=file-prefix-2/
ANOTHER_KEY=other
`
	if err := os.WriteFile(envPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write env file: %v", err)
	}

	reg := inference.NewZeroCostRegistry("")
	if err := reg.LoadFromEnvFile(envPath); err != nil {
		t.Fatalf("failed to load from env file: %v", err)
	}

	if !reg.IsZeroCost("file-model-1") {
		t.Errorf("expected file-model-1 from env file to be zero cost")
	}
	if !reg.IsZeroCost("file-prefix/submodel") {
		t.Errorf("expected file-prefix/submodel to be zero cost")
	}
	if !reg.IsZeroCost("file-prefix-2/submodel") {
		t.Errorf("expected file-prefix-2/submodel to be zero cost")
	}

	// Add new model and save to env file
	reg.AddModel("newly-added-model")
	if err := reg.SaveToEnvFile(envPath); err != nil {
		t.Fatalf("failed to save to env file: %v", err)
	}

	// Reload from saved file into a clean registry
	reg2 := inference.NewZeroCostRegistry("")
	if err := reg2.LoadFromEnvFile(envPath); err != nil {
		t.Fatalf("failed to reload from env file: %v", err)
	}
	if !reg2.IsZeroCost("newly-added-model") {
		t.Errorf("expected newly-added-model to persist in saved .env file")
	}
}

func TestZeroCostRegistry_DiskPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "zero_cost.json")

	reg := inference.NewZeroCostRegistry(jsonPath)
	reg.AddModel("custom-disk-model")
	reg.AddPrefix("custom-disk-prefix/")
	reg.RemoveModel("local-qwen-coder")

	if err := reg.SaveToFile(jsonPath); err != nil {
		t.Fatalf("failed to save to json file: %v", err)
	}

	// Load into fresh registry
	reg2 := inference.NewZeroCostRegistry(jsonPath)
	if err := reg2.LoadFromFile(jsonPath); err != nil {
		t.Fatalf("failed to load from json file: %v", err)
	}

	if !reg2.IsZeroCost("custom-disk-model") {
		t.Errorf("expected custom-disk-model to persist on disk")
	}
	if !reg2.IsZeroCost("custom-disk-prefix/test") {
		t.Errorf("expected custom-disk-prefix/test to persist on disk")
	}
	if reg2.IsZeroCost("local-qwen-coder") {
		t.Errorf("expected removed model local-qwen-coder to stay removed")
	}
}

func TestZeroCostRegistry_Reset(t *testing.T) {
	reg := inference.NewZeroCostRegistry("")
	reg.AddModel("temporary-model")
	reg.RemoveModel("local-qwen3-27b")

	reg.Reset()

	if reg.IsZeroCost("temporary-model") {
		t.Errorf("expected temporary-model to be gone after Reset")
	}
	if !reg.IsZeroCost("local-qwen3-27b") {
		t.Errorf("expected default local-qwen3-27b to be restored after Reset")
	}
}

func TestZeroCostRegistry_Concurrency(t *testing.T) {
	reg := inference.NewZeroCostRegistry("")
	var wg sync.WaitGroup

	// Concurrently read, add, and remove
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			modelName := "dynamic-model"
			reg.IsZeroCost("local-qwen3-27b")
			reg.IsZeroCost("ollama/test")
			reg.AddModel(modelName)
			reg.IsZeroCost(modelName)
			reg.RemoveModel(modelName)
			_ = reg.GetDetails()
		}(i)
	}

	wg.Wait()
}
