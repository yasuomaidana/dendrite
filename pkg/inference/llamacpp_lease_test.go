package inference

import (
	"context"
	"errors"
	"testing"

	"Dendrite/pkg/llamalease"
)

func TestLlamaCppLocalServerLease(t *testing.T) {
	ctx := context.Background()
	local := NewLlamaCppEngine(EngineConfig{ID: "o", Endpoint: "http://127.0.0.1:8080"}, nil)
	oldManager, oldBest := llamaManager, llamaBestFitting
	t.Cleanup(func() { llamaManager, llamaBestFitting = oldManager, oldBest })

	manager := func(healthy bool, fits error) {
		llamaManager = func() *llamalease.Manager {
			return llamalease.New(llamalease.Options{
				Dir:     t.TempDir(),
				Healthy: func() bool { return healthy },
				Fits:    func(llamalease.Request) error { return fits },
			})
		}
	}

	t.Run("autostart off", func(t *testing.T) {
		t.Setenv("DENDRITE_LLAMA_AUTOSTART", "off")
		release, err := local.acquireLocalServer(ctx)
		if err != nil {
			t.Fatal(err)
		}
		release()
	})
	t.Run("a remote endpoint needs no lease", func(t *testing.T) {
		remote := NewLlamaCppEngine(EngineConfig{ID: "o", Endpoint: "http://remote:8080"}, nil)
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
		llamaBestFitting = func(context.Context) (llamalease.Model, error) {
			return llamalease.Model{}, errors.New("none installed")
		}
		if _, err := local.acquireLocalServer(ctx); err == nil {
			t.Fatal("expected the model error")
		}
	})
	t.Run("the server may not start", func(t *testing.T) {
		manager(false, errors.New("too big"))
		llamaBestFitting = func(context.Context) (llamalease.Model, error) {
			return llamalease.Model{ID: "m", Path: "/models/m.gguf"}, nil
		}
		if _, err := local.acquireLocalServer(ctx); !errors.Is(err, llamalease.ErrNotFit) {
			t.Fatalf("err = %v, want ErrNotFit", err)
		}
	})
}

func TestLlamaCppGenerateStopsWhenTheServerCannotBeLeased(t *testing.T) {
	oldManager, oldBest := llamaManager, llamaBestFitting
	t.Cleanup(func() { llamaManager, llamaBestFitting = oldManager, oldBest })
	llamaManager = func() *llamalease.Manager {
		return llamalease.New(llamalease.Options{Dir: t.TempDir(), Healthy: func() bool { return false }, Fits: func(llamalease.Request) error { return errors.New("too big") }})
	}
	llamaBestFitting = func(context.Context) (llamalease.Model, error) { return llamalease.Model{ID: "m"}, nil }
	e := NewLlamaCppEngine(EngineConfig{ID: "o", Endpoint: "http://127.0.0.1:8080"}, nil)
	if _, err := e.Generate(context.Background(), CompletionRequest{Prompt: "hi"}); err == nil {
		t.Fatal("Generate must report that the server cannot start")
	}
}

func TestLlamaCppModelForPrefersTheRequestThenTheConfiguration(t *testing.T) {
	configured := NewLlamaCppEngine(EngineConfig{ID: "l", Models: []string{"first", "second"}}, nil)
	bare := NewLlamaCppEngine(EngineConfig{ID: "l"}, nil)
	for _, c := range []struct {
		engine *LlamaCppEngine
		model  string
		want   string
	}{
		{configured, "asked", "asked"},
		{configured, "", "first"},
		{bare, "", "llamacpp-gguf"},
	} {
		if got := c.engine.modelFor(CompletionRequest{Model: c.model}); got != c.want {
			t.Errorf("modelFor(%q) = %q, want %q", c.model, got, c.want)
		}
	}
}
