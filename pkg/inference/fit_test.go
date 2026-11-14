package inference

import (
	"path/filepath"
	"testing"

	"Dendrite/pkg/memfit"
)

func TestEstimateModelGBReadsParameterCounts(t *testing.T) {
	cases := []struct {
		name string
		want float64
		ok   bool
	}{
		{"hermes3:8b", 4.8, true},
		{"qwen3.8:27b", 16.2, true},
		{"gpt-oss:20b", 12, true},
		{"qwen3-coder:30b-a3b-instruct-q4_K_S", 18, true},
		{"llama3.2:3b", 1.8, true},
		{"qwen3:0.6b", 0.36, true},
		{"nomic-embed-text:latest", 0, false},
		{"tinyllama:latest", 0, false},
		{"mistral-7b-instruct-q4", 4.2, true},
	}
	for _, c := range cases {
		got, ok := EstimateModelGB(c.name)
		if ok != c.ok || (ok && (got < c.want-0.01 || got > c.want+0.01)) {
			t.Errorf("EstimateModelGB(%q) = %.2f, %v; want %.2f, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestFitsRefusesWhatWouldSwapOnThisMachine(t *testing.T) {
	budget := uint64(15 << 30) // the budget of the 24 GB M3 this was measured on, about 62% of RAM
	for name, want := range map[string]bool{
		"hermes3:8b":      true,
		"gpt-oss:20b":     true,  // 12 GB estimated
		"qwen3.8:27b":     false, // the model that put this machine 21 GB into swap
		"qwen3-coder:30b": false,
		"tinyllama":       true, // size unknown: benefit of the doubt
	} {
		if got := FitsBudget(name, budget); got != want {
			t.Errorf("FitsBudget(%q, 15 GB) = %v, want %v", name, got, want)
		}
	}
	t.Setenv(memfit.EnvBudgetGB, "200")
	if !Fits("qwen3.8:27b") {
		t.Error("a budget raised through the environment should admit the 27B model")
	}

}

func TestChatModelResolutionOrder(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv(memfit.EnvBudgetGB, "40") // the stored models fit, whatever the machine running the test has
	t.Setenv(ChatModelEnv, "")
	if got := ChatModel(TierAsk); got != FallbackModel {
		t.Errorf("nothing configured: %q, want the fallback %q", got, FallbackModel)
	}
	if err := (Tiers{Ask: "gpt-oss:20b", Chore: "qwen3:4b"}).Save(); err != nil {
		t.Fatal(err)
	}
	if ChatModel(TierAsk) != "gpt-oss:20b" || ChatModel(TierChore) != "qwen3:4b" {
		t.Errorf("stored tiers: ask=%q chore=%q", ChatModel(TierAsk), ChatModel(TierChore))
	}
	if p, _ := TiersPath(); filepath.Base(p) != "models.json" {
		t.Errorf("tiers path = %q", p)
	}
	t.Setenv(ChatModelEnv+"_CHORE", "llama3.2:3b")
	if ChatModel(TierChore) != "llama3.2:3b" || ChatModel(TierAsk) != "gpt-oss:20b" {
		t.Error("a per-tier override must affect only its tier")
	}
	t.Setenv(ChatModelEnv, "custom:7b")
	if ChatModel(TierAsk) != "custom:7b" {
		t.Error("the global override did not apply")
	}
}

func TestChatModelNeverReturnsAStoredModelThatSwaps(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv(ChatModelEnv, "")
	t.Setenv(memfit.EnvBudgetGB, "0.5") // nothing with a known size fits
	if err := (Tiers{Ask: "qwen3.8:27b", Chore: "qwen3.8:27b"}).Save(); err != nil {
		t.Fatal(err)
	}
	if got := ChatModel(TierAsk); got != FallbackModel {
		t.Errorf("a stored model that does not fit was returned: %q", got)
	}
}
