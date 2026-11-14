package inference

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/oxilease"
)

func TestOllamaRefusesAModelThatWouldStarveTheMachine(t *testing.T) {
	t.Setenv("DENDRITE_MODEL_MEM_BUDGET_GB", "1")
	t.Setenv("DENDRITE_MODEL_GUARD", "")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"big:27b","size":19327352832}]}`))
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	e := NewOllamaEngine(EngineConfig{ID: "o", Endpoint: s.URL}, nil)
	if _, err := e.Generate(context.Background(), CompletionRequest{Prompt: "hi", Model: "big:27b"}); err == nil || !strings.Contains(err.Error(), "memory budget") {
		t.Fatalf("err = %v, want the memory guard", err)
	}
}

func TestOllamaStreamReportsABrokenStream(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{\"response\":\"a\",\"done\":false}\n{broken"))
	}))
	defer s.Close()
	ch, err := NewOllamaEngine(EngineConfig{ID: "o", Endpoint: s.URL, Models: []string{"m"}}, nil).Stream(context.Background(), CompletionRequest{Prompt: "hi", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if text, _, errs := drain(ch); text != "a" || errs != 1 {
		t.Fatalf("text = %q, errors = %d, want the first token and one decode error", text, errs)
	}
}

func TestOllamaStreamRejectsAnUnusableURL(t *testing.T) {
	if _, err := NewOllamaEngine(EngineConfig{ID: "o", Endpoint: "http://%zz"}, nil).Stream(context.Background(), CompletionRequest{Prompt: "hi", Model: "m"}); err == nil {
		t.Fatal("expected a URL error")
	}
}

func TestOxiLLaMaModelDiscoveryFallsBackToTheLocalModelsDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	models := filepath.Join(home, ".oxillama", "models")
	for dir, manifest := range map[string]string{"named": `{"name":"manifest-name"}`, "bare": ""} {
		if err := os.MkdirAll(filepath.Join(models, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if manifest != "" {
			if err := os.WriteFile(filepath.Join(models, dir, "oxillama-manifest.json"), []byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(models, "a-file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	e := NewOxiLLaMaEngine(EngineConfig{ID: "o", Endpoint: closedEndpoint()}, nil)
	got, err := e.SupportedModels(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("models = %v, err = %v", got, err)
	}
	if h := e.Health(context.Background()); h.Reachable {
		t.Errorf("health = %+v", h)
	}
}

func TestOxiLLaMaModelDiscoveryWithNothingInstalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ok := answering(200, `{"data":[{"id":"served"}]}`)
	defer ok.Close()
	if got, err := NewOxiLLaMaEngine(EngineConfig{ID: "o", Endpoint: ok.URL}, nil).SupportedModels(context.Background()); err != nil || len(got) != 1 || got[0] != "served" {
		t.Errorf("models = %v, err = %v", got, err)
	}
	bad := answering(200, "not json")
	defer bad.Close()
	// A reachable server with an unreadable answer and no local models falls back to the configuration.
	got, err := NewOxiLLaMaEngine(EngineConfig{ID: "o", Endpoint: bad.URL, Models: []string{"configured"}}, nil).SupportedModels(context.Background())
	if err != nil || len(got) != 1 || got[0] != "configured" {
		t.Errorf("models = %v, err = %v", got, err)
	}
}

func TestOxiLLaMaLocalServerLease(t *testing.T) {
	ctx := context.Background()
	local := NewOxiLLaMaEngine(EngineConfig{ID: "o", Endpoint: "http://127.0.0.1:8080"}, nil)
	oldManager, oldBest := oxiManager, oxiBestFitting
	t.Cleanup(func() { oxiManager, oxiBestFitting = oldManager, oldBest })

	manager := func(healthy bool, fits error) {
		oxiManager = func() *oxilease.Manager {
			return oxilease.New(oxilease.Options{
				Dir:     t.TempDir(),
				Healthy: func() bool { return healthy },
				Fits:    func(oxilease.Request) error { return fits },
			})
		}
	}

	t.Run("autostart off", func(t *testing.T) {
		t.Setenv("DENDRITE_OXILLAMA_AUTOSTART", "off")
		release, err := local.acquireLocalServer(ctx)
		if err != nil {
			t.Fatal(err)
		}
		release()
	})
	t.Run("a remote endpoint needs no lease", func(t *testing.T) {
		remote := NewOxiLLaMaEngine(EngineConfig{ID: "o", Endpoint: "http://remote:8080"}, nil)
		if _, err := remote.acquireLocalServer(ctx); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a running server is leased and released", func(t *testing.T) {
		manager(true, nil)
		release, err := local.acquireLocalServer(ctx)
		if err != nil {
			t.Fatal(err)
		}
		release()
	})
	t.Run("no installed model fits", func(t *testing.T) {
		manager(false, nil)
		oxiBestFitting = func(context.Context) (oxilease.Model, error) { return oxilease.Model{}, errors.New("none installed") }
		if _, err := local.acquireLocalServer(ctx); err == nil {
			t.Fatal("expected the model error")
		}
	})
	t.Run("the server may not start", func(t *testing.T) {
		manager(false, errors.New("too big"))
		oxiBestFitting = func(context.Context) (oxilease.Model, error) {
			return oxilease.Model{ID: "m", Path: "/models/m.gguf"}, nil
		}
		if _, err := local.acquireLocalServer(ctx); !errors.Is(err, oxilease.ErrNotFit) {
			t.Fatalf("err = %v, want ErrNotFit", err)
		}
	})
}

func TestOxiLLaMaGenerateStopsWhenTheServerCannotBeLeased(t *testing.T) {
	oldManager, oldBest := oxiManager, oxiBestFitting
	t.Cleanup(func() { oxiManager, oxiBestFitting = oldManager, oldBest })
	oxiManager = func() *oxilease.Manager {
		return oxilease.New(oxilease.Options{Dir: t.TempDir(), Healthy: func() bool { return false }, Fits: func(oxilease.Request) error { return errors.New("too big") }})
	}
	oxiBestFitting = func(context.Context) (oxilease.Model, error) { return oxilease.Model{ID: "m"}, nil }
	e := NewOxiLLaMaEngine(EngineConfig{ID: "o", Endpoint: "http://127.0.0.1:8080"}, nil)
	if _, err := e.Generate(context.Background(), CompletionRequest{Prompt: "hi"}); err == nil {
		t.Fatal("Generate must report that the server cannot start")
	}
}

func TestOxiLLaMaModelDiscoveryReportsAnUnreachableServerWithNothingInstalled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	e := NewOxiLLaMaEngine(EngineConfig{ID: "o", Endpoint: closedEndpoint()}, nil)
	if _, err := e.SupportedModels(context.Background()); err == nil {
		t.Fatal("expected the connection error")
	}
}
