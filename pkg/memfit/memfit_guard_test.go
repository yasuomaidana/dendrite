package memfit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckStartAtCountsWhatOllamaHoldsWhenStartingAnotherEngine(t *testing.T) {
	t.Setenv(EnvBudgetGB, "12")
	ollama, _ := fakeEngines(t, `{"name":"big:27b","size":19327352832}`)
	ctx := context.Background()

	cases := []struct {
		name    string
		engine  string
		size    int64
		guard   string
		wantErr bool
	}{
		{"oxillama next to a loaded 18 GB Ollama model is refused", "oxillama", 5 * gib, "", true},
		{"ollama itself ignores its own loaded models", "ollama", 5 * gib, "", false},
		{"a model above the whole budget is refused", "ollama", 20 * gib, "", true},
		{"the guard switch lets everything through", "oxillama", 5 * gib, "off", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvGuard, c.guard)
			err := CheckStartAt(ctx, ollama, c.engine, c.size)
			if c.wantErr && !errors.Is(err, ErrTooLarge) {
				t.Fatalf("err = %v, want ErrTooLarge", err)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}
}

func TestCheckStartUsesTheDefaultOllamaEndpoint(t *testing.T) {
	t.Setenv(EnvBudgetGB, "12")
	t.Setenv(EnvGuard, "")
	if err := CheckStart(context.Background(), "ollama", 1*gib); err != nil {
		t.Fatalf("a 1 GB model within a 12 GB budget was refused: %v", err)
	}
}

func TestTotalRAMBytesIsReadOnSupportedSystems(t *testing.T) {
	got := TotalRAMBytes()
	supported := runtime.GOOS == "darwin" || runtime.GOOS == "linux"
	if supported && got == 0 {
		t.Fatalf("TotalRAMBytes = 0 on %s", runtime.GOOS)
	}
	if !supported && got != 0 {
		t.Fatalf("TotalRAMBytes = %d on unsupported %s", got, runtime.GOOS)
	}
}

func TestBudgetFallsBackToAShareOfRAMWhenTheOverrideIsInvalid(t *testing.T) {
	for _, raw := range []string{"", "not-a-number", "-3", "0"} {
		t.Setenv(EnvBudgetGB, raw)
		got := BudgetBytes()
		if total := TotalRAMBytes(); total > 0 {
			if want := total / 100 * DefaultBudgetPercent; got != want {
				t.Errorf("%q: budget = %d, want %d", raw, got, want)
			}
		} else if got != 4<<30 {
			t.Errorf("%q: budget = %d, want the 4 GB floor", raw, got)
		}
	}
}

func TestGetJSONReportsRequestAndStatusErrors(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer bad.Close()

	var out map[string]any
	cases := []struct{ name, url string }{
		{"a malformed URL", "://bad"},
		{"a server error", bad.URL},
		{"an unreachable server", "http://127.0.0.1:1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := getJSON(context.Background(), http.DefaultClient, c.url, &out); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestGGUFSizeMatchesByNameOrFallsBackToTheOnlyModel(t *testing.T) {
	write := func(home, dir string, size int64) {
		t.Helper()
		d := filepath.Join(home, ".oxillama", "models", dir)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(d, "model.gguf"))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}

	t.Run("no home directory", func(t *testing.T) {
		t.Setenv("HOME", "")
		if got := ggufSize("qwen3-8b"); got != 0 {
			t.Fatalf("size = %d, want 0", got)
		}
	})
	t.Run("nothing installed", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		if got := ggufSize("qwen3-8b"); got != 0 {
			t.Fatalf("size = %d, want 0", got)
		}
	})
	t.Run("a single model is used even when the name differs", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		write(home, "other_model", 7)
		if got := ggufSize("qwen3-8b"); got != 7 {
			t.Fatalf("size = %d, want 7", got)
		}
	})
	t.Run("several models need a name match", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		write(home, "a_model", 3)
		write(home, "qwen3_8b", 9)
		if got := ggufSize("Qwen3-8B"); got != 9 {
			t.Fatalf("matched size = %d, want 9", got)
		}
		if got := ggufSize("unknown"); got != 0 {
			t.Fatalf("unmatched size = %d, want 0", got)
		}
	})
}
