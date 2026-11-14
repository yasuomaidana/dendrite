package inference

// Local engines are ordered GPU-capable Rust runtimes first (candle, Hugging Face TGI/TEI),
// then Ollama, vLLM and llama.cpp, with OxiLLaMa last. oxillama-runtime (0.1.4) offloads
// only Q4_0 single-token decode to the GPU: prompt prefill and embeddings run on the CPU,
// measured at over 3 s per embedded chunk where Ollama on Metal needs about 0.1 s.

// enginePreference ranks engine types; lower runs first among equally capable models.
var enginePreference = map[EngineType]int{
	EngineCandle:         0,
	EngineHuggingFaceTGI: 1,
	EngineHuggingFaceTEI: 1,
	EngineOllama:         2,
	EngineVLLM:           3,
	EngineLlamaCpp:       4,
	EngineOxiLLaMa:       9,
}

// EnginePreference returns the rank of an engine type (lower is preferred). Unknown
// types rank after every known GPU-capable engine and before OxiLLaMa.
func EnginePreference(t EngineType) int {
	if p, ok := enginePreference[t]; ok {
		return p
	}
	return 5
}

// GPUAccelerated reports whether an engine runs prompt prefill on a GPU when one is
// present. OxiLLaMa does not.
func GPUAccelerated(t EngineType) bool {
	return t != EngineOxiLLaMa
}
