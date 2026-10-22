package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const gatewayInfo = `{"data": [
 {"model_name": "gemini-2.5-pro", "litellm_params": {"model": "gemini/gemini-2.5-pro"}, "model_info": {"mode": "chat", "input_cost_per_token": 1.25e-06}},
 {"model_name": "local-qwen-coder", "litellm_params": {"model": "openai/qwen2.5-coder-7b", "api_base": "http://host.docker.internal:8080/v1"}, "model_info": {"mode": "chat", "input_cost_per_token": 0}},
 {"model_name": "local-hermes3-8b", "litellm_params": {"model": "ollama/hermes3:8b", "api_base": "http://host.docker.internal:11434"}, "model_info": {"mode": "chat", "input_cost_per_token": 0}},
 {"model_name": "local-coder", "litellm_params": {"model": "ollama/hermes3:8b", "api_base": "http://host.docker.internal:11434"}, "model_info": {"mode": "chat", "input_cost_per_token": 0}},
 {"model_name": "local-nomic-embed-text-latest", "litellm_params": {"model": "ollama/nomic-embed-text:latest", "api_base": "http://host.docker.internal:11434"}, "model_info": {"mode": "chat", "input_cost_per_token": 0}},
 {"model_name": "hf-deepseek", "litellm_params": {"model": "openai/deepseek", "api_base": "http://tgi-node:8080/v1"}, "model_info": {"mode": "chat", "input_cost_per_token": 0}}
]}`

func gatewayModels(t *testing.T) []gatewayModel {
	t.Helper()
	var info struct {
		Data []gatewayModel `json:"data"`
	}
	if err := json.Unmarshal([]byte(gatewayInfo), &info); err != nil {
		t.Fatal(err)
	}
	return info.Data
}

func clearEndpoints(t *testing.T) {
	for _, v := range []string{"DENDRITE_OLLAMA_ENDPOINT", "DENDRITE_OXILLAMA_ENDPOINT", "DENDRITE_HF_TGI_ENDPOINT", "DENDRITE_VLLM_ENDPOINT", "DENDRITE_LLAMACPP_ENDPOINT", "DENDRITE_CANDLE_ENDPOINT", "DENDRITE_HF_TEI_ENDPOINT"} {
		t.Setenv(v, "")
	}
}

func TestPickLocalPrefersReachableGPUEngines(t *testing.T) {
	clearEndpoints(t)
	models := gatewayModels(t)
	all := func(string) bool { return true }
	if got, ok := pickLocal(models, all); !ok || got.Model != "hf-deepseek" || got.Engine != "huggingface_tgi" {
		t.Fatalf("with every engine up, TGI (GPU server) ranks first: %+v %v", got, ok)
	}
	onlyLocal := func(addr string) bool { return addr == "127.0.0.1:11434" || addr == "127.0.0.1:8080" }
	got, ok := pickLocal(models, onlyLocal)
	if !ok || got.Model != "local-coder" || got.Engine != "ollama" {
		t.Fatalf("an unreachable GPU server is skipped, Ollama wins, OxiLLaMa never: %+v %v", got, ok)
	}
	if _, ok := pickLocal(models, func(addr string) bool { return addr == "127.0.0.1:8080" }); ok {
		t.Fatal("OxiLLaMa (CPU prefill) alone is no GPU choice")
	}
}

func TestPreferLocalModelAsksTheGateway(t *testing.T) {
	clearEndpoints(t)
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/model/info" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(gatewayInfo))
	}))
	defer srv.Close()
	t.Setenv("LITELLM_PROXY_URL", srv.URL+"/v1")
	// The Ollama backend is this test server: reachable.
	t.Setenv("DENDRITE_OLLAMA_ENDPOINT", "http://host.docker.internal:11434")
	_, _ = PreferLocalModel(context.Background(), "sk-test")
	if auth != "Bearer sk-test" {
		t.Fatalf("authorization = %q", auth)
	}
	t.Setenv("LITELLM_PROXY_URL", "http://127.0.0.1:1")
	if _, ok := PreferLocalModel(context.Background(), ""); ok {
		t.Fatal("an unreachable gateway yields no choice")
	}
}
