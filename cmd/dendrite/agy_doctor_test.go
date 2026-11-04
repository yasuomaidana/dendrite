package main

import (
	"Dendrite/pkg/env/bootstrap"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDiagnoseAGYReportsEachState(t *testing.T) {
	cases := []struct {
		state bootstrap.AGYMCPState
		err   error
		want  string
	}{
		{bootstrap.AGYAbsent, nil, "not installed"},
		{bootstrap.AGYRegistered, nil, "is registered"},
		{bootstrap.AGYNotRegistered, nil, "soma prepare"},
		{bootstrap.AGYUnreadable, errors.New("boom"), "cannot read its MCP registry (boom)"},
	}
	old := agyMCPStatus
	t.Cleanup(func() { agyMCPStatus = old })
	for _, c := range cases {
		agyMCPStatus = func(name string) (bootstrap.AGYMCPState, error) {
			if name != "dendrite" {
				t.Errorf("asked about %q", name)
			}
			return c.state, c.err
		}
		var out bytes.Buffer
		diagnoseAGY(&out)
		if !strings.Contains(out.String(), c.want) {
			t.Errorf("state %v: %q, want %q", c.state, out.String(), c.want)
		}
	}
}

func TestRegisterWithAGYLines(t *testing.T) {
	old := agyRegister
	t.Cleanup(func() { agyRegister = old })
	agyRegister = func(string) ([]string, string) { return []string{"dendrite", "dendrite-synapse-x"}, "" }
	if got := registerWithAGY("."); len(got) != 1 || !strings.Contains(got[0], "dendrite, dendrite-synapse-x") {
		t.Errorf("lines = %v", got)
	}
	agyRegister = func(string) ([]string, string) { return nil, "agy mcp add dendrite: exit 1" }
	if got := registerWithAGY("."); len(got) != 1 || !strings.Contains(got[0], "exit 1") {
		t.Errorf("lines = %v", got)
	}
	agyRegister = func(string) ([]string, string) { return nil, "" }
	if got := registerWithAGY("."); len(got) != 0 {
		t.Errorf("AGY absent must be silent, got %v", got)
	}
}

func TestSomaPrepareRegistersWithAGYUnlessItIsADryRun(t *testing.T) {
	var targets []string
	old := agyRegister
	t.Cleanup(func() { agyRegister = old })
	agyRegister = func(target string) ([]string, string) {
		targets = append(targets, target)
		return []string{"dendrite"}, ""
	}
	dir := t.TempDir()
	opts := somaPrepareOptions{Path: dir, Tool: "copilot", NoCICD: true, NoSkills: true}

	var out bytes.Buffer
	if err := runSomaPrepare(context.Background(), opts, &out); err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0] != dir || !strings.Contains(out.String(), "[✓] AGY: registered MCP servers dendrite") {
		t.Errorf("targets %v, output:\n%s", targets, out.String())
	}

	opts.DryRun = true
	out.Reset()
	if err := runSomaPrepare(context.Background(), opts, &out); err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || strings.Contains(out.String(), "registered MCP servers") {
		t.Errorf("a dry run must not register: targets %v, output:\n%s", targets, out.String())
	}
}
