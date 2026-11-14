package inference

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/memfit"
)

func fakeOllama(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:14b","size":10000000000},{"name":"hermes3:8b","size":5033164800},{"name":"qwen3:4b","size":2684354560},{"name":"nomic-embed-text:latest","size":274000000}]}`))
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func withOxiModel(t *testing.T, bytes int64) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".oxillama", "models", "qwen3_8b_q4km")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Create(filepath.Join(dir, "model.gguf"))
	_ = f.Truncate(bytes)
	_ = f.Close()
	_ = os.WriteFile(filepath.Join(dir, "id"), []byte("qwen3-8b-q4_k_m\n"), 0o644)
}

func TestBestLocalModelPrefersTheSmartestThatFits(t *testing.T) {
	withOxiModel(t, 5<<30)
	ollama := fakeOllama(t)

	t.Setenv("DENDRITE_MODEL_MEM_BUDGET_GB", "12")
	got, err := BestLocalModel(context.Background(), ollama, "http://127.0.0.1:1")
	if err != nil || got.Model != "qwen3:14b" || got.Engine != "ollama" {
		t.Fatalf("12 GB: %+v, %v; want qwen3:14b on ollama", got, err)
	}

	// 14B no longer fits; the stopped OxiLLaMa model (73) outranks hermes3:8b (67) and is started by a lease.
	t.Setenv("DENDRITE_MODEL_MEM_BUDGET_GB", "6.5")
	got, err = BestLocalModel(context.Background(), ollama, "http://127.0.0.1:1")
	if err != nil || got.Model != "qwen3-8b-q4_k_m" || got.Engine != "oxillama" || !got.NeedsStart {
		t.Fatalf("6.5 GB: %+v, %v; want oxillama qwen3-8b-q4_k_m needing a start", got, err)
	}

	// Only the small one fits.
	t.Setenv("DENDRITE_MODEL_MEM_BUDGET_GB", "3.5")
	got, err = BestLocalModel(context.Background(), ollama, "http://127.0.0.1:1")
	if err != nil || got.Model != "qwen3:4b" {
		t.Fatalf("3.5 GB: %+v, %v; want qwen3:4b", got, err)
	}

	// Nothing fits: an error naming the command, never a model that would starve the machine.
	t.Setenv("DENDRITE_MODEL_MEM_BUDGET_GB", "1")
	if got, err := BestLocalModel(context.Background(), ollama, "http://127.0.0.1:1"); err == nil {
		t.Fatalf("1 GB: got %+v, want an error", got)
	}
}

func TestResolveOllamaUsesTheSameRanking(t *testing.T) {
	t.Setenv("DENDRITE_MODEL_MEM_BUDGET_GB", "12")
	ollama := fakeOllama(t)
	// hermes3:8b is larger than qwen3:4b but qwen3:14b scores highest and fits.
	if got := memfit.ResolveOllama(context.Background(), ollama, "gone:70b"); got != "qwen3:14b" {
		t.Errorf("resolved to %q, want qwen3:14b", got)
	}
}
