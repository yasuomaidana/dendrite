package inference

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestModelSizeAndEngineHelpers(t *testing.T) {
	if _, ok := EstimateModelGB("model-0b"); ok {
		t.Error("a zero-parameter tag has no estimate")
	}
	if endpointFor("not-an-engine") != "" {
		t.Error("an unknown engine type has no endpoint")
	}
	for in, want := range map[string]string{
		"http://%zz":             "",
		"no-host":                "",
		"https://example.com":    "example.com:443",
		"http://example.com":     "example.com:80",
		"http://localhost:11434": "127.0.0.1:11434",
	} {
		if got := HostAddress(in); got != want {
			t.Errorf("HostAddress(%q) = %q, want %q", in, got, want)
		}
	}
	if _, ok := EngineAt(""); ok {
		t.Error("an empty endpoint belongs to no engine")
	}
	if got := EnginePreference("mystery"); got != 5 {
		t.Errorf("an unknown engine ranks %d, want 5", got)
	}
}

func TestPickBestPrefersScoreThenOllamaThenSize(t *testing.T) {
	cases := []struct {
		name  string
		cands []Choice
		want  string
	}{
		{"a higher score wins", []Choice{{Model: "a", Score: 1}, {Model: "b", Score: 3}, {Model: "c", Score: 2}}, "b"},
		{"on equal scores Ollama beats OxiLLaMa", []Choice{{Model: "oxi", Engine: "oxillama", Score: 2}, {Model: "oll", Engine: "ollama", Score: 2}}, "oll"},
		{"an engine other than Ollama does not displace the first", []Choice{{Model: "first", Engine: "ollama", Score: 2}, {Model: "oxi", Engine: "oxillama", Score: 2}}, "first"},
		{"on the same engine the larger model wins", []Choice{{Model: "small", Engine: "ollama", Score: 2, SizeBytes: 1}, {Model: "large", Engine: "ollama", Score: 2, SizeBytes: 9}}, "large"},
	}
	for _, c := range cases {
		if got := pickBest(c.cands); got.Model != c.want {
			t.Errorf("%s: pickBest = %q, want %q", c.name, got.Model, c.want)
		}
	}
}

func TestTiersReportStorageProblems(t *testing.T) {
	t.Run("no home directory", func(t *testing.T) {
		t.Setenv("DENDRITE_HOME", "")
		t.Setenv("HOME", "")
		if _, err := TiersPath(); err == nil {
			t.Error("TiersPath needs a home")
		}
		if _, err := LoadTiers(); err == nil {
			t.Error("LoadTiers needs a home")
		}
		if err := (Tiers{}).Save(); err == nil {
			t.Error("Save needs a home")
		}
	})
	t.Run("the file is a directory", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("DENDRITE_HOME", home)
		if err := os.Mkdir(filepath.Join(home, "models.json"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadTiers(); err == nil {
			t.Error("a directory cannot be read as tiers")
		}
		if err := (Tiers{Ask: "m"}).Save(); err == nil {
			t.Error("a directory cannot be written as tiers")
		}
	})
	t.Run("the home is a file", func(t *testing.T) {
		parent := t.TempDir()
		file := filepath.Join(parent, "home")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("DENDRITE_HOME", filepath.Join(file, "sub"))
		if err := (Tiers{}).Save(); err == nil {
			t.Error("a home that cannot be created fails")
		}
	})
}

func TestManagerHelpers(t *testing.T) {
	if e, err := NewEngineFromConfig(EngineConfig{Type: EngineCandle, Endpoint: "http://127.0.0.1:1"}, nil); err != nil || e.Type() != EngineCandle {
		t.Errorf("candle config = %v, %v", e, err)
	}
	t.Setenv("DENDRITE_TEST_LIST", " , ")
	if got := getInferenceEnvSliceOrDefault("DENDRITE_TEST_LIST", []string{"fallback"}); len(got) != 1 || got[0] != "fallback" {
		t.Errorf("a list of blanks falls back, got %v", got)
	}
	m := NewManager()
	m.cascadeOrder = []string{"unregistered"}
	if _, err := m.ExecuteWithCascade(context.Background(), CompletionRequest{Prompt: "hi"}); err == nil {
		t.Error("a cascade of unknown engines has none to use")
	}
}

func TestQueueSkipsWorkWhoseContextEndedWhileWaiting(t *testing.T) {
	q := NewLocalLLMQueue(QueueConfig{MaxConcurrency: 1, MaxQueueSize: 4, DefaultTimeout: time.Second})
	defer q.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ran := false
	item := &queueItem{ctx: ctx, done: make(chan error, 1), task: func(context.Context) error { ran = true; return nil }}
	q.execute(item)
	if err := <-item.done; err == nil || ran {
		t.Fatalf("err = %v, ran = %v; a cancelled item must be skipped with its context's error", err, ran)
	}
	if q.Metrics().TotalTimedOut != 1 {
		t.Errorf("metrics = %+v", q.Metrics())
	}
}

func TestQueuedEngineReturnsTheEnginesFailure(t *testing.T) {
	q := NewLocalLLMQueue(QueueConfig{MaxConcurrency: 1, MaxQueueSize: 4, DefaultTimeout: time.Second})
	defer q.Close()
	e := NewQueuedInferenceEngine(NewVLLMEngine(EngineConfig{ID: "down", Endpoint: closedEndpoint()}, nil), q, PriorityNormal)
	if _, err := e.Generate(context.Background(), CompletionRequest{Prompt: "hi"}); err == nil {
		t.Error("Generate must return the engine's failure")
	}
	if _, err := e.Stream(context.Background(), CompletionRequest{Prompt: "hi"}); err == nil {
		t.Error("Stream must return the engine's failure")
	}
}

// sseServer streams the given lines and then closes without a final marker.
func sseServer(lines ...string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for _, l := range lines {
			_, _ = w.Write([]byte(l + "\n\n"))
		}
	}))
}

func drain(ch <-chan StreamChunk) (text string, done bool, errs int) {
	for c := range ch {
		text += c.Text
		done = done || c.Done
		if c.Error != nil {
			errs++
		}
	}
	return text, done, errs
}

func TestStreamsEndWithoutAFinalMarkerAndSkipJunkLines(t *testing.T) {
	openAI := []string{"data: not json", `data: {"choices":[]}`, `data: {"choices":[{"delta":{"content":"he"}}]}`, `data: {"choices":[{"delta":{"content":"llo"}}]}`}
	tgi := []string{"data: not json", `data: {"token":{"text":"he"},"generated_text":null}`, `data: {"token":{"text":"llo"},"generated_text":null}`}
	llama := []string{"data: not json", `data: {"content":"he","stop":false}`, `data: {"content":"llo","stop":false}`}

	cases := []struct {
		name  string
		lines []string
		make  func(url string) InferenceEngine
	}{
		{"vllm", openAI, func(u string) InferenceEngine {
			return NewVLLMEngine(EngineConfig{ID: "v", Endpoint: u, APIKey: "k"}, nil)
		}},
		{"oxillama", openAI, func(u string) InferenceEngine { return NewOxiLLaMaEngine(EngineConfig{ID: "o", Endpoint: u}, nil) }},
		{"tgi", tgi, func(u string) InferenceEngine {
			return NewHuggingFaceTGIEngine(EngineConfig{ID: "t", Endpoint: u}, nil)
		}},
		{"llamacpp", llama, func(u string) InferenceEngine { return NewLlamaCppEngine(EngineConfig{ID: "l", Endpoint: u}, nil) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := sseServer(c.lines...)
			defer s.Close()
			ch, err := c.make(s.URL).Stream(context.Background(), CompletionRequest{Prompt: "hi"})
			if err != nil {
				t.Fatal(err)
			}
			text, _, errs := drain(ch)
			if text != "hello" || errs != 0 {
				t.Fatalf("text = %q, errors = %d", text, errs)
			}
		})
	}
}

func TestStreamReportsAnAnswerCutShort(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n"))
	}))
	defer s.Close()
	ch, err := NewVLLMEngine(EngineConfig{ID: "v", Endpoint: s.URL}, nil).Stream(context.Background(), CompletionRequest{Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, errs := drain(ch); errs != 1 {
		t.Fatalf("errors = %d, want the read failure reported once", errs)
	}
}

func TestOptionalRequestFieldsReachTheServer(t *testing.T) {
	var body string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		_, _ = w.Write([]byte(`{"content":"x","tokens_evaluated":0,"tokens_predicted":0,"generated_text":"x"}`))
	}))
	defer s.Close()
	if _, err := NewLlamaCppEngine(EngineConfig{ID: "l", Endpoint: s.URL}, nil).Generate(context.Background(), CompletionRequest{Prompt: "hi", Stop: []string{"END"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"stop":["END"]`) {
		t.Errorf("stop sequences missing from %s", body)
	}
}

func TestPostJSONRequestReportsABodyItCannotEncode(t *testing.T) {
	if _, err := postJSONRequest(context.Background(), http.DefaultClient, "http://127.0.0.1:1", make(chan int)); err == nil {
		t.Fatal("expected an encoding error")
	}
}

func TestHuggingFaceAnswersMayBeOneObjectOrTokenCountsMayBeMissing(t *testing.T) {
	ctx := context.Background()
	one := answering(200, `{"generated_text":"single"}`)
	defer one.Close()
	res, err := NewHuggingFaceEndpointEngine(EngineConfig{ID: "e", Endpoint: one.URL, Models: []string{"m"}}, nil).Generate(ctx, CompletionRequest{Prompt: "hi"})
	if err != nil || res.Text != "single" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	bad := answering(200, `"neither"`)
	defer bad.Close()
	if _, err := NewHuggingFaceEndpointEngine(EngineConfig{ID: "e", Endpoint: bad.URL, Models: []string{"m"}}, nil).Generate(ctx, CompletionRequest{Prompt: "hi"}); err == nil {
		t.Error("an answer of the wrong shape must be an error")
	}
	tgi := answering(200, `{"generated_text":"words here","details":{"generated_tokens":2}}`)
	defer tgi.Close()
	res, err = NewHuggingFaceTGIEngine(EngineConfig{ID: "t", Endpoint: tgi.URL}, nil).Generate(ctx, CompletionRequest{Prompt: "count these words"})
	if err != nil || res.PromptTokens != 3 {
		t.Fatalf("res = %+v, err = %v; prompt tokens fall back to a word count", res, err)
	}
}
