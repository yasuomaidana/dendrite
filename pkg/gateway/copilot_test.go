package gateway

import "testing"

func TestCopilotModel(t *testing.T) {
	t.Setenv("DENDRITE_COPILOT_MODEL", "")
	if got := CopilotModel(); got != DefaultCopilotModel {
		t.Fatalf("CopilotModel() = %q; want default %q", got, DefaultCopilotModel)
	}
	t.Setenv("DENDRITE_COPILOT_MODEL", "gpt-5.5")
	if got := CopilotModel(); got != "gpt-5.5" {
		t.Fatalf("CopilotModel() = %q; want override gpt-5.5", got)
	}
}
