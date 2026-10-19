package main

import (
	"strings"
	"testing"
)

func TestCognition(t *testing.T) {
	useTempStore(t)
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"cognition", "policy"}, ExitOK, "Token-Aware Hybrid Scheduler Policy:"},
		{[]string{"cognition", "policy", "--night-window", "22:00-06:00", "--weekly-reset", "MON 09:00", "--sprint-before-reset", "24h", "--reserve-floor", "0.3", "--allow-zero", "--mode", "auto"}, ExitOK, "Successfully updated"},
		{[]string{"analyze", "policy", "--json"}, ExitOK, `"night_window": "22:00-06:00"`},
		{[]string{"cognition", "policy", "--reserve-floor", "1.5"}, ExitUsage, ""},
		{[]string{"cognition", "policy", "--sprint-before-reset", "soon"}, ExitUsage, ""},
		{[]string{"cognition", "status"}, ExitOK, "Token-Aware Hybrid Scheduler & Quota Governor Status"},
		{[]string{"cognition", "status", "--json"}, ExitOK, `"governor"`},
		{[]string{"cognition", "switch", "--allow-zero", "on"}, ExitOK, "Allow-Zero Drain Override: ON"},
		{[]string{"cognition", "switch", "--allow-zero", "off"}, ExitOK, "Allow-Zero Drain Override: OFF"},
		{[]string{"cognition", "switch", "--allow-zero", "maybe"}, ExitUsage, ""},
		{[]string{"cognition", "switch"}, ExitUsage, ""},
		{[]string{"cognition", "mode"}, ExitOK, "Current Operating Mode:"},
		{[]string{"cognition", "mode", "always"}, ExitOK, "mode to: always"},
		{[]string{"cognition", "mode", "pause"}, ExitOK, "mode to: paused"},
		{[]string{"cognition", "mode", "policy"}, ExitOK, "mode to: policy"},
		{[]string{"cognition", "mode", "turbo"}, ExitUsage, ""},
		{[]string{"cognition", "pause", "--project", "demo", "--chunk", "c1"}, ExitOK, "paused"},
		{[]string{"cognition", "resume", "--project", "demo", "-c", "c1"}, ExitOK, "resumed"},
		{[]string{"cognition", "run", "--simulate-night"}, ExitOK, "Chunk Execution Outcome"},
		{[]string{"cognition", "run", "--simulate-sprint", "--json"}, ExitOK, "{"},
		{[]string{"cognition", "run", "--simulate-night", "--simulate-sprint"}, ExitUsage, ""},
		{[]string{"cognition", "tokens"}, ExitUsage, ""},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
}

func TestParseOnOff(t *testing.T) {
	for in, want := range map[string]bool{"on": true, "TRUE": true, "1": true, "yes": true, "off": false, "0": false, "disable": false} {
		if got, err := parseOnOff(in); err != nil || got != want {
			t.Errorf("parseOnOff(%q) = %v, %v", in, got, err)
		}
	}
}
