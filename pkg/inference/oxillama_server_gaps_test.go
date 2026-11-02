package inference

import (
	"Dendrite/pkg/testkit"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// withoutFlush hides the Flush method of a ResponseWriter.
type withoutFlush struct{ http.ResponseWriter }

func freePort(t *testing.T) int { return testkit.FreePort(t) }

func TestOxiLLaMaServerDefaultsAndLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	d := NewOxiLLaMaServer(OxiLLaMaServerConfig{})
	if d.cfg.Host != "127.0.0.1" || d.cfg.Port != 8080 || d.cfg.ModelsDir != filepath.Join(home, ".oxillama", "models") {
		t.Fatalf("defaults = %+v", d.cfg)
	}

	s := NewOxiLLaMaServer(OxiLLaMaServerConfig{Port: freePort(t), ModelsDir: t.TempDir()})
	if err := s.Stop(context.Background()); err != nil {
		t.Fatalf("stopping a server that never started: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("starting twice: %v", err)
	}
	if !s.IsRunning() {
		t.Fatal("server should be running")
	}

	other := NewOxiLLaMaServer(OxiLLaMaServerConfig{Port: s.cfg.Port, ModelsDir: t.TempDir()})
	if err := other.Start(); err == nil {
		t.Error("binding a taken port must fail")
	}
	if err := s.Stop(context.Background()); err != nil || s.IsRunning() {
		t.Fatalf("stop: %v, running = %v", err, s.IsRunning())
	}
}

func TestOxiLLaMaServerRejectsBadRequests(t *testing.T) {
	s := NewOxiLLaMaServer(OxiLLaMaServerConfig{ModelsDir: t.TempDir()})
	post := func(h http.HandlerFunc, w http.ResponseWriter, method, body string) {
		h(w, httptest.NewRequest(method, "/x", strings.NewReader(body)))
	}

	rec := httptest.NewRecorder()
	post(s.handleModels, rec, http.MethodPost, "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("models POST = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	post(s.handleChatCompletions, rec, http.MethodGet, "")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("chat GET = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	post(s.handleChatCompletions, rec, http.MethodPost, "{")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("chat with broken JSON = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	post(s.handleChatCompletions, rec, http.MethodPost, `{"model":"no-such-model","messages":[]}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("chat with an unknown model = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	post(s.handleChatCompletions, &withoutFlush{rec}, http.MethodPost, `{"model":"qwen3.5:9b","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("streaming without a flusher = %d", rec.Code)
	}
}

func TestListStoredModels(t *testing.T) {
	builtin := []string{"qwen3.5:9b", "qwen3.8:27b"}
	missing := NewOxiLLaMaServer(OxiLLaMaServerConfig{ModelsDir: filepath.Join(t.TempDir(), "none")})
	if got := missing.listStoredModels(); len(got) != 2 {
		t.Errorf("a missing directory lists %v, want %v", got, builtin)
	}
	empty := NewOxiLLaMaServer(OxiLLaMaServerConfig{ModelsDir: t.TempDir()})
	if got := empty.listStoredModels(); len(got) != 2 {
		t.Errorf("an empty directory lists %v, want %v", got, builtin)
	}

	dir := t.TempDir()
	for _, sub := range []string{"qwen3_8b", "named"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "named", "oxillama-manifest.json"), []byte(`{"name":"from-manifest"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stray-file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got := NewOxiLLaMaServer(OxiLLaMaServerConfig{ModelsDir: dir}).listStoredModels()
	want := map[string]bool{"qwen3:8b": true, "from-manifest": true}
	if len(got) != 2 || !want[got[0]] || !want[got[1]] {
		t.Errorf("models = %v", got)
	}
}

func TestSynthesizedAnswers(t *testing.T) {
	s := NewOxiLLaMaServer(OxiLLaMaServerConfig{ModelsDir: t.TempDir()})
	cases := []struct {
		name   string
		prompt string
		want   []string
	}{
		{"pure rust", "tell me about pure-rust", []string{"zero-FFI GGUF"}},
		{"plain request", "hello there", []string{"zero cloud tokens"}},
		{"commit with a titled ticket", "write a git commit message\n[DEN-12] Add Search\nchanged pkg/devtool/x.go", []string{"[DEN-12] feat(devtool): add Search"}},
		{"commit with a bare ticket", "git diff for DEN-77 touching pkg/gateway/x.go", []string{"[DEN-77] feat(gateway): implement dynamic routing"}},
		{"commit fix", "conventional commit: fix the bug in pkg/rag/x.go", []string{"fix(rag): implement local-first RAG"}},
		{"commit docs", "conventional commit: doc update docs/guide.md", []string{"docs(docs):"}},
		{"commit refactor", "conventional commit: refactor pkg/agent/a.go", []string{"refactor(agent):"}},
		{"commit test", "conventional commit: test pkg/store/a_test.go", []string{"test(store):"}},
		{"commit devtool summary", "conventional commit pkg/devtool/a.go", []string{"smart commit strategy"}},
		{"commit across scopes", "conventional commit pkg/env/a.go pkg/tracker/b.go cmd/dendrite/c.go", []string{"- Update and synchronize env capabilities", "synchronize cli"}},
		{"commit with no scope", "conventional commit please", []string{"feat(repo):"}},
		{"retrieved commands", "Retrieved Project Context:\n$ dendrite work new X\n`dendrite sync`\ndendrite work new X\ndendrite ask q\ndendrite a <b>\ndendrite c\ndendrite d\ndendrite e\ndendrite f", []string{"1. `$ dendrite work new X`", "2. `$ dendrite c`", "5. `$ dendrite f`"}},
		{"retrieved model listing", "--- PROJECT CONTEXT ---\nhow do I check available LLMs", []string{"dendrite spike list"}},
		{"retrieved nothing useful", "Retrieved Project Context:\nnothing", []string{"Processed via 'm' with retrieved project context."}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := s.synthesizeResponse("m", c.prompt)
			for _, want := range c.want {
				if !strings.Contains(got, want) {
					t.Errorf("response lacks %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestOxiLLaMaServerStreamsTheAnswer(t *testing.T) {
	s := NewOxiLLaMaServer(OxiLLaMaServerConfig{Port: freePort(t), ModelsDir: t.TempDir()})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop(context.Background())
	e := NewOxiLLaMaEngine(EngineConfig{ID: "o", Endpoint: "http://127.0.0.1:" + strconv.Itoa(s.cfg.Port)}, nil)
	t.Setenv("DENDRITE_OXILLAMA_AUTOSTART", "off")
	ch, err := e.Stream(context.Background(), CompletionRequest{Prompt: "pure-rust please", Model: "qwen3.5:9b"})
	if err != nil {
		t.Fatal(err)
	}
	if text, done, _ := drain(ch); !done || !strings.Contains(text, "zero-FFI") {
		t.Fatalf("streamed %q (done %v)", text, done)
	}
}
