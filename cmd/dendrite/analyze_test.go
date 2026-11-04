package main

import (
	"Dendrite/cmd/dendrite/internal/cli"
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
		{[]string{"cognition", "policy"}, cli.ExitOK, "Token-Aware Hybrid Scheduler Policy:"},
		{[]string{"cognition", "policy", "--night-window", "22:00-06:00", "--weekly-reset", "MON 09:00", "--sprint-before-reset", "24h", "--reserve-floor", "0.3", "--allow-zero", "--mode", "auto"}, cli.ExitOK, "Successfully updated"},
		{[]string{"analyze", "policy", "--json"}, cli.ExitOK, `"night_window": "22:00-06:00"`},
		{[]string{"cognition", "policy", "--reserve-floor", "1.5"}, cli.ExitUsage, ""},
		{[]string{"cognition", "policy", "--sprint-before-reset", "soon"}, cli.ExitUsage, ""},
		{[]string{"cognition", "status"}, cli.ExitOK, "Token-Aware Hybrid Scheduler & Quota Governor Status"},
		{[]string{"cognition", "status", "--json"}, cli.ExitOK, `"governor"`},
		{[]string{"cognition", "switch", "--allow-zero", "on"}, cli.ExitOK, "Allow-Zero Drain Override: ON"},
		{[]string{"cognition", "switch", "--allow-zero", "off"}, cli.ExitOK, "Allow-Zero Drain Override: OFF"},
		{[]string{"cognition", "switch", "--allow-zero", "maybe"}, cli.ExitUsage, ""},
		{[]string{"cognition", "switch"}, cli.ExitUsage, ""},
		{[]string{"cognition", "mode"}, cli.ExitOK, "Current Operating Mode:"},
		{[]string{"cognition", "mode", "always"}, cli.ExitOK, "mode to: always"},
		{[]string{"cognition", "mode", "pause"}, cli.ExitOK, "mode to: paused"},
		{[]string{"cognition", "mode", "policy"}, cli.ExitOK, "mode to: policy"},
		{[]string{"cognition", "mode", "turbo"}, cli.ExitUsage, ""},
		{[]string{"cognition", "pause", "--project", "demo", "--chunk", "c1"}, cli.ExitOK, "paused"},
		{[]string{"cognition", "resume", "--project", "demo", "-c", "c1"}, cli.ExitOK, "resumed"},
		{[]string{"cognition", "run", "--simulate-night"}, cli.ExitOK, "Chunk Execution Outcome"},
		{[]string{"cognition", "run", "--simulate-sprint", "--json"}, cli.ExitOK, "{"},
		{[]string{"cognition", "run", "--simulate-night", "--simulate-sprint"}, cli.ExitUsage, ""},
		{[]string{"cognition", "tokens"}, cli.ExitUsage, ""},
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
