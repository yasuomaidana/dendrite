package modelcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func serve(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func gatewayModels(t *testing.T) []Model {
	t.Helper()
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/model/info" || r.Header.Get("Authorization") != "Bearer sk-1" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":[
		 {"model_name":"flash","litellm_params":{"model":"gemini/gemini-3.8-flash"},"model_info":{"mode":"chat","input_cost_per_token":7.5e-8,"output_cost_per_token":3e-7,"max_input_tokens":1048576}},
		 {"model_name":"local-coder","litellm_params":{"model":"ollama/hermes3:8b"},"model_info":{"mode":"chat","input_cost_per_token":0}}]}`))
	})
	got, err := GatewaySource{URL: url + "/v1", Key: "sk-1"}.List(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("list = %+v, %v", got, err)
	}
	return got
}

func TestGatewaySourceReadsPricesAndContext(t *testing.T) {
	flash := gatewayModels(t)[0]
	want := Model{Name: "flash", Provider: "gemini", Upstream: "gemini-3.8-flash", Mode: "chat", InputCost: 7.5e-8,
		OutputCost: 3e-7, MaxInput: 1048576, Routed: true, Source: "gateway"}
	if flash != want {
		t.Errorf("flash = %+v, want %+v", flash, want)
	}
}

func TestGatewaySourceMarksZeroCostModelsLocal(t *testing.T) {
	if l := gatewayModels(t)[1]; !l.Local || l.Provider != "ollama" {
		t.Errorf("local = %+v", l)
	}
}

// geminiServer serves two pages of models and records whether the key ever appeared in a URL.
func geminiServer(t *testing.T, keyInURL *bool) string {
	t.Helper()
	return serve(t, func(w http.ResponseWriter, r *http.Request) {
		*keyInURL = *keyInURL || strings.Contains(r.URL.RawQuery, "sk-gem")
		if r.Header.Get("x-goog-api-key") != "sk-gem" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{"models":[
			 {"name":"models/gemini-4-argon-medium","inputTokenLimit":1048576,"supportedGenerationMethods":["generateContent"]},
			 {"name":"models/gemini-embedding-001","supportedGenerationMethods":["embedContent"]},
			 {"name":"models/gemini-5-preview","supportedGenerationMethods":["generateContent"]}],"nextPageToken":"p2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-3.8-flash-tts","supportedGenerationMethods":["generateContent"]},
		 {"name":"models/gemini-3.8-flash","supportedGenerationMethods":["generateContent","countTokens"]}]}`))
	})
}

func TestGeminiSourceKeepsTextModelsAcrossPagesAndKeepsTheKeyOutOfTheURL(t *testing.T) {
	var keyInURL bool
	url := geminiServer(t, &keyInURL)
	got, err := GeminiSource{Key: "sk-gem", BaseURL: url}.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if names := nameList(got); names != "gemini-4-argon-medium,gemini-3.8-flash" {
		t.Errorf("models = %s", names)
	}
	if keyInURL {
		t.Error("the key travelled in the URL")
	}
}

func TestGeminiSourcePreviewsAreOptIn(t *testing.T) {
	var keyInURL bool
	url := geminiServer(t, &keyInURL)
	preview, err := GeminiSource{Key: "sk-gem", BaseURL: url, Preview: true}.List(context.Background())
	if err != nil || !strings.Contains(nameList(preview), "gemini-5-preview") {
		t.Errorf("Preview did not keep preview models: %v, %v", nameList(preview), err)
	}
}

func TestGeminiSourceKeyProblems(t *testing.T) {
	var keyInURL bool
	url := geminiServer(t, &keyInURL)
	if _, err := (GeminiSource{BaseURL: url}).List(context.Background()); err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Errorf("no key: err = %v", err)
	}
	if _, err := (GeminiSource{Key: "wrong", BaseURL: url}).List(context.Background()); err == nil || strings.Contains(err.Error(), "wrong") {
		t.Errorf("bad key: err = %v (must not echo the key)", err)
	}
}

func nameList(models []Model) string {
	var names []string
	for _, m := range models {
		names = append(names, m.Name)
	}
	return strings.Join(names, ",")
}

func TestCopilotSourceKeepsChatModelsInThePicker(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("Copilot-Integration-Id") == "" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":[
		 {"id":"claude-sonnet-4.6","model_picker_enabled":true,"capabilities":{"type":"chat","limits":{"max_context_window_tokens":200000}}},
		 {"id":"text-embedding-3","capabilities":{"type":"embeddings"}},
		 {"id":"hidden","model_picker_enabled":false,"capabilities":{"type":"chat"}}]}`))
	})
	got, err := CopilotSource{Token: "tok", BaseURL: url}.List(context.Background())
	if err != nil || nameList(got) != "claude-sonnet-4.6" || got[0].MaxInput != 200000 || got[0].Provider != providerCopilot {
		t.Fatalf("list = %+v, %v", got, err)
	}
	if _, err := (CopilotSource{BaseURL: url}).List(context.Background()); err == nil {
		t.Error("no token was accepted")
	}
}

func TestAGYAndOllamaSources(t *testing.T) {
	agy, err := AGYSource{ListFn: func(context.Context) ([]string, error) { return []string{"gemini-3.8-flash-low"}, nil }}.List(context.Background())
	if err != nil || len(agy) != 1 || agy[0].Provider != "agy" {
		t.Fatalf("agy = %+v, %v", agy, err)
	}
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:4b"},{"name":"hermes3:8b"}]}`))
	})
	local, err := OllamaSource{URL: url}.List(context.Background())
	if err != nil || nameList(local) != "qwen3:4b,hermes3:8b" || !local[0].Local || local[0].Routed {
		t.Fatalf("ollama = %+v, %v", local, err)
	}
	if _, err := (OllamaSource{URL: "http://127.0.0.1:1"}).List(context.Background()); err == nil {
		t.Error("an unreachable Ollama was not an error")
	}
}

func TestParseAGY(t *testing.T) {
	out := "gemini-3.8-flash-low\tFlash (low)\nclaude-opus-4.6\tOpus\nheader line\n"
	if got := strings.Join(ParseAGY(out), ","); got != "claude-opus-4.6,gemini-3.8-flash-low" {
		t.Errorf("models = %s", got)
	}
}

func TestSourcesReportWhyTheyFailWithoutLeakingSecrets(t *testing.T) {
	status := serve(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusUnauthorized) })
	garbage := serve(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>")) })
	ctx := context.Background()
	for _, c := range []struct {
		name string
		list func() ([]Model, error)
		want string
	}{
		{"gateway status", func() ([]Model, error) { return GatewaySource{URL: status, Key: "sk-secret"}.List(ctx) }, "HTTP 401"},
		{"gateway garbage", func() ([]Model, error) { return GatewaySource{URL: garbage}.List(ctx) }, "unreadable"},
		{"gateway down", func() ([]Model, error) { return GatewaySource{URL: "http://127.0.0.1:1"}.List(ctx) }, "unreachable"},
		{"copilot status", func() ([]Model, error) { return CopilotSource{Token: "tok-secret", BaseURL: status}.List(ctx) }, "HTTP 401"},
		{"agy", func() ([]Model, error) {
			return AGYSource{ListFn: func(context.Context) ([]string, error) { return nil, os.ErrNotExist }}.List(ctx)
		}, "agy models failed"},
	} {
		_, err := c.list()
		if err == nil || !strings.Contains(err.Error(), c.want) || strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: err = %v, want %q and no secret", c.name, err, c.want)
		}
	}
}

func TestSourceNames(t *testing.T) {
	for want, s := range map[string]Source{"gateway": GatewaySource{}, "ollama": OllamaSource{}, "agy": AGYSource{}, "gemini": GeminiSource{}, "copilot": CopilotSource{}} {
		if s.Name() != want {
			t.Errorf("%T.Name() = %q, want %q", s, s.Name(), want)
		}
	}
}

func TestAnUnparsableURLIsAnErrorNotAPanic(t *testing.T) {
	if _, err := (GatewaySource{URL: "http://bad host"}).List(context.Background()); err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Errorf("err = %v", err)
	}
}

func TestBaseURL(t *testing.T) {
	for _, c := range []struct{ url, def, want string }{
		{"", "https://x", "https://x"},
		{"/", "https://x", "https://x"},
		{"http://localhost:1234/", "https://x", "http://localhost:1234"},
	} {
		if got := baseURL(c.url, c.def); got != c.want {
			t.Errorf("baseURL(%q, %q) = %q, want %q", c.url, c.def, got, c.want)
		}
	}
}
