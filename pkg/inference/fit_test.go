package inference

import (
	"path/filepath"
	"testing"
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
	const ram = 24.0 // the M3 MacBook Air this was measured on
	for name, want := range map[string]bool{
		"hermes3:8b":      true,
		"gpt-oss:20b":     true,  // 12 GB estimated, 14 GB on disk: under 65% of 24 GB (15.6 GB)
		"qwen3.8:27b":     false, // the model that put this machine 21 GB into swap
		"qwen3-coder:30b": false, // 18 GB: fits only after the GPU memory limit is raised and the fraction too
		"tinyllama":       true,  // size unknown: benefit of the doubt
	} {
		if got := Fits(name, ram, 0); got != want {
			t.Errorf("Fits(%q, 24 GB) = %v, want %v", name, got, want)
		}
	}
	if !Fits("qwen3-coder:30b", ram, 0.8) {
		t.Error("an 80% fraction should admit the 30B mixture-of-experts model")
	}
	if !Fits("qwen3.8:27b", 0, 0) {
		t.Error("unknown RAM must not reject models")
	}
	if !Fits("qwen3.8:27b", 64, 0) {
		t.Error("a 64 GB machine fits a 27B model")
	}
}

func TestFitFractionOverrideIsValidated(t *testing.T) {
	t.Setenv(FitFractionEnv, "0.8")
	if FitFraction() != 0.8 {
		t.Errorf("FitFraction = %v", FitFraction())
	}
	for _, bad := range []string{"", "x", "0", "1.5", "-1"} {
		t.Setenv(FitFractionEnv, bad)
		if FitFraction() != DefaultFitFraction {
			t.Errorf("%q gave %v, want the default", bad, FitFraction())
		}
	}
}

func TestChatModelResolutionOrder(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
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
	t.Setenv(FitFractionEnv, "0.01") // nothing with a known size fits
	if err := (Tiers{Ask: "qwen3.8:27b", Chore: "qwen3.8:27b"}).Save(); err != nil {
		t.Fatal(err)
	}
	if got := ChatModel(TierAsk); got != FallbackModel && MachineRAMGB() > 0 {
		t.Errorf("a stored model that does not fit was returned: %q", got)
	}
}
