package modelcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/ollama"
)

// Source lists the models one place knows about.
type Source interface {
	Name() string
	List(ctx context.Context) ([]Model, error)
}

const (
	listTimeout = 10 * time.Second
	maxBody     = 8 << 20
)

// getJSON fetches url with headers and decodes the JSON answer. Errors never include the URL's query
// or the headers, which may carry a key.
func getJSON(ctx context.Context, client *http.Client, url string, headers map[string]string, out any) error {
	if client == nil {
		client = &http.Client{Timeout: listTimeout}
	}
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("bad request")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return fmt.Errorf("unreadable answer")
	}
	return nil
}

// GatewaySource lists what the LiteLLM gateway routes, with prices and context sizes.
type GatewaySource struct {
	URL, Key string
	Client   *http.Client
}

func (GatewaySource) Name() string { return "gateway" }

type gatewayEntry struct {
	ModelName string `json:"model_name"`
	Params    struct {
		Model string `json:"model"`
	} `json:"litellm_params"`
	Info struct {
		Mode       string   `json:"mode"`
		InputCost  *float64 `json:"input_cost_per_token"`
		OutputCost *float64 `json:"output_cost_per_token"`
		MaxInput   int      `json:"max_input_tokens"`
	} `json:"model_info"`
}

// splitUpstream turns "gemini/gemini-4" into provider and model id.
func splitUpstream(s string) (provider, id string) {
	if i := strings.Index(s, "/"); i > 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

func (g GatewaySource) List(ctx context.Context) ([]Model, error) {
	var res struct {
		Data []gatewayEntry `json:"data"`
	}
	headers := map[string]string{}
	if g.Key != "" {
		headers["Authorization"] = "Bearer " + g.Key
	}
	url := strings.TrimSuffix(strings.TrimRight(g.URL, "/"), "/v1") + "/v1/model/info"
	if err := getJSON(ctx, g.Client, url, headers, &res); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(res.Data))
	for _, e := range res.Data {
		provider, id := splitUpstream(e.Params.Model)
		m := Model{Name: e.ModelName, Provider: provider, Upstream: id, Mode: e.Info.Mode,
			MaxInput: e.Info.MaxInput, Routed: true, Source: "gateway"}
		if e.Info.InputCost != nil {
			m.InputCost = *e.Info.InputCost
		}
		if e.Info.OutputCost != nil {
			m.OutputCost = *e.Info.OutputCost
		}
		m.Local = e.Info.InputCost != nil && *e.Info.InputCost == 0
		models = append(models, m)
	}
	return models, nil
}

// baseURL is url without a trailing slash, or def when url is empty.
func baseURL(url, def string) string {
	if url = strings.TrimRight(url, "/"); url != "" {
		return url
	}
	return def
}

// OllamaSource lists the models installed in Ollama. It only reads; nothing is pulled.
type OllamaSource struct{ URL string }

func (OllamaSource) Name() string { return "ollama" }

func (o OllamaSource) List(ctx context.Context) ([]Model, error) {
	infos, err := ollama.NewClient(ollama.WithBaseURL(o.URL), ollama.WithTimeout(listTimeout)).ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("unreachable")
	}
	models := make([]Model, 0, len(infos))
	for _, i := range infos {
		models = append(models, Model{Name: i.Name, Provider: "ollama", Upstream: i.Name, Local: true, Mode: "chat", Source: "ollama"})
	}
	return models, nil
}

// AGYSource lists the models the Antigravity CLI offers; list runs `agy models`.
type AGYSource struct {
	ListFn func(ctx context.Context) ([]string, error)
}

func (AGYSource) Name() string { return "agy" }

func (a AGYSource) List(ctx context.Context) ([]Model, error) {
	ids, err := a.ListFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("agy models failed")
	}
	models := make([]Model, 0, len(ids))
	for _, id := range ids {
		models = append(models, Model{Name: id, Provider: "agy", Upstream: id, Mode: "chat", Source: "agy"})
	}
	return models, nil
}

// notChat names Gemini models that are not text chat models.
var notChat = regexp.MustCompile(`embed|tts|image|aqa|imagen|veo|audio`)

// GeminiSource lists the Gemini models that can generate text.
type GeminiSource struct {
	Key, BaseURL string
	// Preview keeps preview and experimental models.
	Preview bool
	Client  *http.Client
}

func (GeminiSource) Name() string { return "gemini" }

func (g GeminiSource) List(ctx context.Context) ([]Model, error) {
	if g.Key == "" {
		return nil, fmt.Errorf("no GEMINI_API_KEY")
	}
	base := baseURL(g.BaseURL, "https://generativelanguage.googleapis.com")
	var models []Model
	token := ""
	for page := 0; page < 5; page++ {
		var res struct {
			Models []struct {
				Name       string   `json:"name"`
				InputLimit int      `json:"inputTokenLimit"`
				Methods    []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			Next string `json:"nextPageToken"`
		}
		url := base + "/v1beta/models?pageSize=200"
		if token != "" {
			url += "&pageToken=" + token
		}
		if err := getJSON(ctx, g.Client, url, map[string]string{"x-goog-api-key": g.Key}, &res); err != nil {
			return nil, err
		}
		for _, m := range res.Models {
			id := strings.TrimPrefix(m.Name, "models/")
			if g.keep(id, m.Methods) {
				models = append(models, Model{Name: id, Provider: "gemini", Upstream: id, MaxInput: m.InputLimit, Mode: "chat", Source: "gemini"})
			}
		}
		if res.Next == "" {
			break
		}
		token = res.Next
	}
	return models, nil
}

func (g GeminiSource) keep(id string, methods []string) bool {
	can := false
	for _, m := range methods {
		can = can || m == "generateContent"
	}
	if !can || notChat.MatchString(id) {
		return false
	}
	return g.Preview || !(strings.Contains(id, "preview") || strings.Contains(id, "-exp"))
}

// CopilotSource lists the chat models GitHub Copilot serves to the signed-in account.
type CopilotSource struct {
	Token, BaseURL string
	Client         *http.Client
}

func (CopilotSource) Name() string { return "copilot" }

func (c CopilotSource) List(ctx context.Context) ([]Model, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("no Copilot token")
	}
	base := baseURL(c.BaseURL, "https://api.githubcopilot.com")
	var res struct {
		Data []struct {
			ID           string `json:"id"`
			PickerOn     *bool  `json:"model_picker_enabled"`
			Capabilities struct {
				Type   string `json:"type"`
				Limits struct {
					Context int `json:"max_context_window_tokens"`
				} `json:"limits"`
			} `json:"capabilities"`
		} `json:"data"`
	}
	headers := map[string]string{
		"Authorization":          "Bearer " + c.Token,
		"Copilot-Integration-Id": "vscode-chat",
		"Editor-Version":         "vscode/1.95.0",
	}
	if err := getJSON(ctx, c.Client, base+"/models", headers, &res); err != nil {
		return nil, err
	}
	var models []Model
	for _, d := range res.Data {
		if d.Capabilities.Type != "chat" || (d.PickerOn != nil && !*d.PickerOn) {
			continue
		}
		models = append(models, Model{Name: d.ID, Provider: providerCopilot, Upstream: d.ID,
			MaxInput: d.Capabilities.Limits.Context, Mode: "chat", Source: "copilot"})
	}
	return models, nil
}
