package inference

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
