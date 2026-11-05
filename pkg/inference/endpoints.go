package inference

import (
	"net"
	"net/url"
	"strings"
)

// defaultEndpoint is where an engine type listens unless its DENDRITE_*_ENDPOINT variable
// says otherwise (DefaultManager registers the same endpoints).
type defaultEndpoint struct {
	Type     EngineType
	Env      string
	Fallback string
}

var defaultEndpoints = []defaultEndpoint{
	{EngineOllama, "DENDRITE_OLLAMA_ENDPOINT", "http://127.0.0.1:11434"},
	{EngineOxiLLaMa, "DENDRITE_OXILLAMA_ENDPOINT", "http://127.0.0.1:8080"},
	{EngineVLLM, "DENDRITE_VLLM_ENDPOINT", "http://vllm-node:8000"},
	{EngineHuggingFaceTGI, "DENDRITE_HF_TGI_ENDPOINT", "http://tgi-node:8080"},
	{EngineHuggingFaceTEI, "DENDRITE_HF_TEI_ENDPOINT", "http://localhost:8082"},
	{EngineCandle, "DENDRITE_CANDLE_ENDPOINT", "http://127.0.0.1:8083"},
	{EngineLlamaCpp, "DENDRITE_LLAMACPP_ENDPOINT", "http://localhost:8081"},
}

// endpointFor returns the endpoint of an engine type (environment, then default).
func endpointFor(t EngineType) string {
	for _, d := range defaultEndpoints {
		if d.Type == t {
			return getInferenceEnvOrDefault(d.Env, d.Fallback)
		}
	}
	return ""
}

// thisMachine reports whether host names the machine Dendrite runs on, as seen from the
// host or from a container of the gateway stack.
func thisMachine(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1", "host.docker.internal", "0.0.0.0":
		return true
	}
	return false
}

// HostAddress is endpoint's host:port as reachable from this machine: hosts that name this
// machine from inside a container become 127.0.0.1. Empty when endpoint has no host.
func HostAddress(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return ""
	}
	host, port := u.Hostname(), u.Port()
	if port == "" {
		port = map[string]string{"https": "443"}[u.Scheme]
		if port == "" {
			port = "80"
		}
	}
	if thisMachine(host) {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// EngineAt returns the engine type that serves endpoint (a gateway backend's api_base), by
// matching it against the engines' endpoints; hosts that name this machine match by port.
func EngineAt(endpoint string) (EngineType, bool) {
	addr := HostAddress(endpoint)
	if addr == "" {
		return "", false
	}
	for _, d := range defaultEndpoints {
		if HostAddress(endpointFor(d.Type)) == addr {
			return d.Type, true
		}
	}
	return "", false
}
