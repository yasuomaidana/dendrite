package main

import (
	"strings"
	"testing"
)

func TestBouton_SearchRouteSessions(t *testing.T) {
	t.Setenv("DENDRITE_STATE_DIR", t.TempDir())
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"bouton", "search", "refactor", "AST", "parser", "--local", "--pkg", "pkg/ast", "--max-cost", "1"}, ExitOK, "Agent Search Results"},
		{[]string{"agent", "search", "refactor", "--json"}, ExitOK, "["},
		{[]string{"bouton", "route", "add", "a", "flag", "--pkg", "cmd/dendrite"}, ExitOK, "Optimal Agent Routing Decision"},
		{[]string{"bouton", "route", "--task", "add a flag", "--json"}, ExitOK, "{"},
		{[]string{"bouton", "sessions"}, ExitOK, "Agent Sessions Ledger (0 recorded sessions)"},
		{[]string{"bouton", "session", "--ticket", "DEN-1", "--json"}, ExitOK, "["},
		{[]string{"bouton", "search"}, ExitUsage, ""},
		{[]string{"bouton", "route"}, ExitUsage, ""},
		{[]string{"bouton", "route", "x", "--task", "y"}, ExitUsage, ""},
		{[]string{"bouton", "run"}, ExitUsage, ""},
		{[]string{"bouton", "run", "clippy"}, ExitUsage, ""},
		{[]string{"bouton", "site"}, ExitUsage, ""},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
}

func TestDefaultAgentModel(t *testing.T) {
	if defaultAgentModel("agy") != "gemini-3.8-flash-high" || defaultAgentModel("aider") != "copilot-sonnet" {
		t.Fatal("unexpected default models")
	}
	if defaultAgentModel("copilot") == "" {
		t.Fatal("copilot needs a model")
	}
}
