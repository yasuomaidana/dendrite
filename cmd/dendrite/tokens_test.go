package main

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"path/filepath"
	"strings"
	"testing"
)

func TestVesicle(t *testing.T) {
	t.Setenv("LITELLM_PROXY_URL", "http://127.0.0.1:1")
	dir := t.TempDir()
	spend := filepath.Join(dir, "spend.json")
	policy := filepath.Join(dir, "policy.json")
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"vesicle", "status", "--spend-file", spend, "--policy-file", policy}, cli.ExitOK, ""},
		{[]string{"tokens", "status", "--spend-file", spend, "--policy-file", policy, "--day-tokens", "500", "--week-tokens", "3500", "--model", "m", "--json"}, cli.ExitOK, "{"},
		{[]string{"vesicle", "status", "--day-tokens", "0"}, cli.ExitUsage, ""},
		{[]string{"vesicle", "status", "--day-tokens", "many"}, cli.ExitUsage, ""},
		{[]string{"vesicle", "estimate", "four", "word", "prompt", "here"}, cli.ExitOK, "Prompt Word Count:       4"},
		{[]string{"vesicle", "audit-prompt", "hello", "--json"}, cli.ExitOK, "{"},
		{[]string{"vesicle", "estimate"}, cli.ExitUsage, ""},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
}
