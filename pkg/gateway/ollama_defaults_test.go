package gateway

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRenderYAMLTunesOllamaChatModelsOnly(t *testing.T) {
	cfg := &LiteLLMConfig{ModelList: []ModelListEntry{
		{ModelName: "local-coder", LitellmParams: ModelParams{Model: "ollama/hermes3:8b", CustomLLMProvider: "ollama", APIBase: "http://h:11434", MaxTokens: 4096}},
		{ModelName: "local-embed", LitellmParams: ModelParams{Model: "ollama/nomic-embed-text:latest", CustomLLMProvider: "ollama", APIBase: "http://h:11434"}},
		{ModelName: "custom", LitellmParams: ModelParams{Model: "ollama/x", CustomLLMProvider: "ollama", APIBase: "http://h:11434", NumCtx: 2048, KeepAlive: "5m"}},
		{ModelName: "llamacpp", LitellmParams: ModelParams{Model: "openai/m", CustomLLMProvider: "openai", APIBase: "http://h:8081/v1", APIKey: "EMPTY"}},
	}}
	data, err := RenderYAML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		ModelList []struct {
			ModelName     string `yaml:"model_name"`
			LitellmParams struct {
				NumCtx    int    `yaml:"num_ctx"`
				KeepAlive string `yaml:"keep_alive"`
				MaxTokens int    `yaml:"max_tokens"`
			} `yaml:"litellm_params"`
		} `yaml:"model_list"`
	}
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string][2]any{
		"local-coder": {OllamaNumCtx, OllamaKeepAlive},
		"local-embed": {0, ""},
		"custom":      {2048, "5m"},
		"llamacpp":    {0, ""},
	}
	for _, m := range got.ModelList {
		w := want[m.ModelName]
		if m.LitellmParams.NumCtx != w[0].(int) || m.LitellmParams.KeepAlive != w[1].(string) {
			t.Errorf("%s: num_ctx=%d keep_alive=%q, want %v", m.ModelName, m.LitellmParams.NumCtx, m.LitellmParams.KeepAlive, w)
		}
	}
	if got.ModelList[0].LitellmParams.MaxTokens != 4096 {
		t.Error("the model's own max_tokens must stay: only ask caps its answers")
	}
	if cfg.ModelList[0].LitellmParams.NumCtx != 0 || strings.Contains(string(data), "num_ctx: 0") {
		t.Error("rendering changed the caller's configuration or wrote a zero")
	}
}
