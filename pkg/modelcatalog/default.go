package modelcatalog

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"Dendrite/pkg/dotenv"
	"Dendrite/pkg/projectid"
)

// Default returns the manager for this machine: the Dendrite home's catalog, fed by the gateway,
// Ollama, `agy models`, and the Gemini and Copilot listings when their keys are configured. Callers
// that can ask for the gateway's local model set LocalPicker.
func Default() (*Manager, error) {
	home, err := projectid.HomeDir()
	if err != nil {
		return nil, err
	}
	m := New(home)
	key := func(name string) string { return dotenv.Lookup(home, name) }
	gateway := envOr("http://localhost:4000", "LITELLM_PROXY_URL")
	ollamaURL := envOr("localhost:11434", "DENDRITE_OLLAMA_ENDPOINT", "OLLAMA_HOST")
	if !strings.Contains(ollamaURL, "://") {
		ollamaURL = "http://" + ollamaURL
	}
	m.Sources = []Source{
		GatewaySource{URL: gateway, Key: key("LITELLM_MASTER_KEY")},
		OllamaSource{URL: ollamaURL},
		AGYSource{ListFn: func(ctx context.Context) ([]string, error) { return ListAGY(ctx, "") }},
		GeminiSource{Key: key("GEMINI_API_KEY"), Preview: m.IncludePreview()},
		CopilotSource{Token: key("COPILOT_API_KEY")},
	}
	m.Spawn = SpawnSelf
	return m, nil
}

// envOr returns the first of the named environment variables that is set, else def.
func envOr(def string, names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return def
}

var agyModelLine = regexp.MustCompile(`^([a-z0-9][a-z0-9.\-]+)\t`)

// ParseAGY reads the model ids from the output of `agy models`, sorted.
func ParseAGY(out string) []string {
	var models []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if m := agyModelLine.FindStringSubmatch(sc.Text()); m != nil {
			models = append(models, m[1])
		}
	}
	sort.Strings(models)
	return models
}

// ListAGY runs `agy models` (bin defaults to agy) and returns the model ids it lists.
func ListAGY(ctx context.Context, bin string) ([]string, error) {
	if bin == "" {
		bin = "agy"
	}
	out, err := exec.CommandContext(ctx, bin, "models").Output()
	if err != nil {
		return nil, fmt.Errorf("agy models failed: %w", err)
	}
	models := ParseAGY(string(out))
	if len(models) == 0 {
		return nil, fmt.Errorf("agy models listed no models")
	}
	return models, nil
}
