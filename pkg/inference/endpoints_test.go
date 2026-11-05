package inference

import "testing"

func TestEngineAtMatchesGatewayBackends(t *testing.T) {
	t.Setenv("DENDRITE_OLLAMA_ENDPOINT", "")
	t.Setenv("DENDRITE_OXILLAMA_ENDPOINT", "")
	t.Setenv("DENDRITE_HF_TGI_ENDPOINT", "")
	for endpoint, want := range map[string]EngineType{
		"http://host.docker.internal:11434":   EngineOllama,
		"http://localhost:11434":              EngineOllama,
		"http://host.docker.internal:8080/v1": EngineOxiLLaMa,
		"http://tgi-node:8080/v1":             EngineHuggingFaceTGI,
		"http://localhost:8081/v1":            EngineLlamaCpp,
	} {
		if got, ok := EngineAt(endpoint); !ok || got != want {
			t.Errorf("EngineAt(%s) = %s, %v; want %s", endpoint, got, ok, want)
		}
	}
	if _, ok := EngineAt("http://elsewhere:9999"); ok {
		t.Error("an unknown endpoint must not match")
	}
	t.Setenv("DENDRITE_OLLAMA_ENDPOINT", "http://gpu-box:11500")
	if got, ok := EngineAt("http://gpu-box:11500/"); !ok || got != EngineOllama {
		t.Errorf("configured endpoint: %s, %v", got, ok)
	}
}
