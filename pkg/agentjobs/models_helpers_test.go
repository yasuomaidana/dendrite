package agentjobs

import "testing"

func TestInputTokensReadsEveryNumberKind(t *testing.T) {
	for name, usage := range map[string]map[string]any{
		"float": {"input_tokens": float64(7)}, "int": {"input_tokens": 7}, "int64": {"input_tokens": int64(7)},
	} {
		if got := inputTokens(usage); got != 7 {
			t.Errorf("%s: %d", name, got)
		}
	}
	if inputTokens(map[string]any{"input_tokens": "7"}) != 0 || inputTokens(nil) != 0 {
		t.Error("a missing or non-numeric count is 0")
	}
}

func TestResumeTokenLimitHonoursTheEnvironment(t *testing.T) {
	t.Setenv("DENDRITE_AGY_RESUME_TOKEN_LIMIT", "1234")
	if resumeTokenLimit() != 1234 {
		t.Errorf("limit = %d", resumeTokenLimit())
	}
	for _, bad := range []string{"", "x", "0", "-5"} {
		t.Setenv("DENDRITE_AGY_RESUME_TOKEN_LIMIT", bad)
		if resumeTokenLimit() != DefaultResumeTokenLimit {
			t.Errorf("%q gave %d", bad, resumeTokenLimit())
		}
	}
}

func TestLiteLLMKeyFallsBackToTheCurrentDirectory(t *testing.T) {
	t.Setenv("LITELLM_MASTER_KEY", "k-from-env")
	if got := (&Dispatcher{}).liteLLMKey(""); got != "k-from-env" {
		t.Errorf("key = %q", got)
	}
}
