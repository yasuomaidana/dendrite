package inference

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"Dendrite/pkg/projectid"
)

// Tier names a job a local chat model does.
type Tier string

const (
	// TierAsk answers questions about the project: quality matters, a minute of waiting is acceptable.
	TierAsk Tier = "ask"
	// TierChore writes commit messages and digests test output: short outputs, speed matters.
	TierChore Tier = "chore"
)

// FallbackModel is used when nothing is configured; it is small enough for any developer machine.
const FallbackModel = "hermes3:8b"

// Tiers maps each tier to a model. It is written by `dendrite spike bench --save` and read everywhere
// a default model used to be hard-coded, so changing the machine's models is one edit in one place.
type Tiers struct {
	Ask   string `json:"ask,omitempty"`
	Chore string `json:"chore,omitempty"`
}

// TiersPath is where the tiers are stored.
func TiersPath() (string, error) {
	home, err := projectid.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "models.json"), nil
}

// LoadTiers reads the stored tiers; a missing file is an empty configuration.
func LoadTiers() (Tiers, error) {
	var t Tiers
	path, err := TiersPath()
	if err != nil {
		return t, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return t, nil
	}
	if err != nil {
		return t, err
	}
	return t, json.Unmarshal(data, &t)
}

// Save writes the tiers.
func (t Tiers) Save() error {
	path, err := TiersPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(t, "", "  ")
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ChatModelEnv overrides the model of every tier (DENDRITE_LOCAL_CHAT_MODEL), and
// ChatModelEnvFor the model of one (DENDRITE_LOCAL_CHAT_MODEL_ASK / _CHORE).
const ChatModelEnv = "DENDRITE_LOCAL_CHAT_MODEL"

// ChatModel resolves the local chat model for a tier: an exported override, then the stored tier,
// then the other tier (one model may serve both), then FallbackModel. It never returns a model that
// does not fit the machine's memory unless the user chose it explicitly.
func ChatModel(t Tier) string {
	if v := strings.TrimSpace(os.Getenv(ChatModelEnv + "_" + strings.ToUpper(string(t)))); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv(ChatModelEnv)); v != "" {
		return v
	}
	stored, _ := LoadTiers()
	candidates := []string{stored.Ask, stored.Chore}
	if t == TierChore {
		candidates = []string{stored.Chore, stored.Ask}
	}
	for _, c := range candidates {
		if c != "" && Fits(c) {
			return c
		}
	}
	return FallbackModel
}
