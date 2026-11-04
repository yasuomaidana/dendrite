package main

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/agentjobs"
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
		{[]string{"bouton", "search", "refactor", "AST", "parser", "--local", "--pkg", "pkg/ast", "--max-cost", "1"}, cli.ExitOK, "Agent Search Results"},
		{[]string{"agent", "search", "refactor", "--json"}, cli.ExitOK, "["},
		{[]string{"bouton", "route", "add", "a", "flag", "--pkg", "cmd/dendrite"}, cli.ExitOK, "Optimal Agent Routing Decision"},
		{[]string{"bouton", "route", "--task", "add a flag", "--json"}, cli.ExitOK, "{"},
		{[]string{"bouton", "sessions"}, cli.ExitOK, "Agent Sessions Ledger (0 recorded sessions)"},
		{[]string{"bouton", "session", "--ticket", "DEN-1", "--json"}, cli.ExitOK, "["},
		{[]string{"bouton", "search"}, cli.ExitUsage, ""},
		{[]string{"bouton", "route"}, cli.ExitUsage, ""},
		{[]string{"bouton", "route", "x", "--task", "y"}, cli.ExitUsage, ""},
		{[]string{"bouton", "run"}, cli.ExitUsage, ""},
		{[]string{"bouton", "run", "clippy"}, cli.ExitUsage, ""},
		{[]string{"bouton", "site"}, cli.ExitUsage, ""},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
}

func TestDefaultAgentModel(t *testing.T) {
	if want, _ := agentjobs.DefaultDispatcher.PickAGYModel(0); defaultAgentModel("agy") != want || defaultAgentModel("aider") != "copilot-sonnet" {
		t.Fatal("unexpected default models")
	}
	if defaultAgentModel("copilot") == "" {
		t.Fatal("copilot needs a model")
	}
}
