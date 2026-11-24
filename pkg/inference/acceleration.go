package inference

// Local engines are ordered GPU-capable Rust runtimes first (candle, Hugging Face TGI/TEI),
// then Ollama, vLLM and llama.cpp.

// enginePreference ranks engine types; lower runs first among equally capable models.
var enginePreference = map[EngineType]int{
	EngineCandle:         0,
	EngineHuggingFaceTGI: 1,
	EngineHuggingFaceTEI: 1,
	EngineOllama:         2,
	EngineVLLM:           3,
	EngineLlamaCpp:       4,
}

// EnginePreference returns the rank of an engine type (lower is preferred). Unknown
// types rank after every known engine.
func EnginePreference(t EngineType) int {
	if p, ok := enginePreference[t]; ok {
		return p
	}
	return 5
}
