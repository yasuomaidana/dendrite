package memfit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gib = 1 << 30

func TestFitsKeepsAKVAllowanceAndCountsOtherLoadedModels(t *testing.T) {
	budget := uint64(12 * gib)
	if ok, _ := Fits(9*gib, budget, 0); !ok {
		t.Error("a 9 GB model should fit a 12 GB budget on its own")
	}
	if ok, why := Fits(9*gib, budget, 4*gib); ok || !strings.Contains(why, "4.0 GB held by other loaded models") {
		t.Errorf("9 GB next to 4 GB resident = %v %q, want a refusal explaining the resident share", ok, why)
	}
	if ok, _ := Fits(13*gib, budget, 0); ok {
		t.Error("13 GB must not fit 12 GB (the gpt-oss:20b case)")
	}
	if ok, _ := Fits(0, budget, 99*gib); !ok {
		t.Error("an unknown size is allowed")
	}
}

func TestBudgetEnvOverrideAndDefault(t *testing.T) {
	t.Setenv(EnvBudgetGB, "7.5")
	if got := BudgetBytes(); got != uint64(7.5*gib) {
		t.Errorf("budget = %d, want 7.5 GB", got)
	}
	t.Setenv(EnvBudgetGB, "")
	if total := TotalRAMBytes(); total > 0 && BudgetBytes() != total/100*DefaultBudgetPercent {
		t.Errorf("default budget = %d, want %d%% of RAM", BudgetBytes(), DefaultBudgetPercent)
	}
}

// fakeEngines serves Ollama (tags + ps) and OxiLLaMa (/v1/models) like the real ones.
func fakeEngines(t *testing.T, loaded string) (ollama, oxi string) {
	t.Helper()
	o := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"big:27b","size":19327352832},{"name":"mid:14b","size":9663676416},{"name":"small:4b","size":2684354560},{"name":"nomic-embed-text:latest","size":274000000}]}`))
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[` + loaded + `]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	x := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"qwen3-8b-q4_k_m"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(o.Close)
	t.Cleanup(x.Close)
	return o.URL, x.URL
}

func withGGUF(t *testing.T, bytes int64) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".oxillama", "models", "qwen3_8b_q4km")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "model.gguf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(bytes); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

func TestAssessMarksWhichInstalledModelsFitAlongsideOxiLLaMa(t *testing.T) {
	t.Setenv(EnvBudgetGB, "12")
	withGGUF(t, 5*gib) // OxiLLaMa holds 5 GB
	ollama, oxi := fakeEngines(t, "")
	r := Assess(context.Background(), ollama, oxi)

	want := map[string]bool{"big:27b": false, "mid:14b": false, "small:4b": true, "nomic-embed-text:latest": true}
	for name, fits := range want {
		m, ok := r.Find("ollama", name)
		if !ok {
			t.Fatalf("%s not listed in %+v", name, r.Models)
		}
		if m.Fits != fits {
			t.Errorf("%s fits = %v (%s), want %v", name, m.Fits, m.Reason, fits)
		}
	}
	if o, ok := r.Find("oxillama", "qwen3-8b-q4_k_m"); !ok || !o.Fits || !o.Resident || o.SizeBytes != 5*gib {
		t.Errorf("oxillama model = %+v", o)
	}
	if got := r.BestFitting(""); got != "small:4b" {
		t.Errorf("best fitting = %q, want small:4b (the embedder is never a chat pick)", got)
	}
}

func TestGuardRefusesAModelThatWouldStarveTheMachineAndNamesAnAlternative(t *testing.T) {
	t.Setenv(EnvBudgetGB, "12")
	withGGUF(t, 1) // OxiLLaMa effectively empty
	ollama, oxi := fakeEngines(t, "")
	_ = oxi
	err := GuardOllama(context.Background(), ollama, "big:27b")
	if !errors.Is(err, ErrTooLarge) || !strings.Contains(err.Error(), "use mid:14b instead") {
		t.Fatalf("err = %v, want ErrTooLarge naming mid:14b", err)
	}
	if err := GuardOllama(context.Background(), ollama, "small:4b"); err != nil {
		t.Errorf("a small model was refused: %v", err)
	}
	if err := GuardOllama(context.Background(), ollama, "not-installed"); err != nil {
		t.Errorf("an unknown model is Ollama's to reject: %v", err)
	}
	t.Setenv(EnvGuard, "off")
	if err := GuardOllama(context.Background(), ollama, "big:27b"); err != nil {
		t.Errorf("guard off still refused: %v", err)
	}
}

func TestGuardLetsAnAlreadyLoadedModelThrough(t *testing.T) {
	t.Setenv(EnvBudgetGB, "12")
	ollama, _ := fakeEngines(t, `{"name":"big:27b","size":19327352832}`)
	if err := GuardOllama(context.Background(), ollama, "big:27b"); err != nil {
		t.Errorf("a loaded model costs nothing more to call: %v", err)
	}
}

func TestUnreachableEnginesContributeNothing(t *testing.T) {
	r := Assess(context.Background(), "http://127.0.0.1:1", "http://127.0.0.1:1")
	if len(r.Models) != 0 || r.OllamaReachable || r.OxiReachable {
		t.Errorf("report = %+v", r)
	}
	if err := GuardOllama(context.Background(), "http://127.0.0.1:1", "anything"); err != nil {
		t.Errorf("guard must not block when Ollama is down: %v", err)
	}
}

func TestResolveOllamaKeepsAFittingRequestAndReplacesAStaleDefault(t *testing.T) {
	t.Setenv(EnvBudgetGB, "12")
	withGGUF(t, 1)
	ollama, _ := fakeEngines(t, "")
	ctx := context.Background()
	if got := ResolveOllama(ctx, ollama, "small:4b"); got != "small:4b" {
		t.Errorf("a fitting request changed to %q", got)
	}
	for _, stale := range []string{"big:27b", "never-installed:7b", ""} {
		if got := ResolveOllama(ctx, ollama, stale); got != "mid:14b" {
			t.Errorf("ResolveOllama(%q) = %q, want the smartest model that fits (mid:14b)", stale, got)
		}
	}
	if got := ResolveOllama(ctx, "http://127.0.0.1:1", "big:27b"); got != "big:27b" {
		t.Errorf("an unreachable Ollama must leave the request alone, got %q", got)
	}
}
