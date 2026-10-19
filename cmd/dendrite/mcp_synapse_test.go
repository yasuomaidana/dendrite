package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestBuildSynapseMCPServerUnknown(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	if _, err := buildSynapseMCPServer("missing"); err == nil {
		t.Fatal("expected error for an unknown synapse")
	}
}

func TestInter_Commands(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	if out, code := cliOutput(t, "inter", "tools"); code != ExitOK || !strings.Contains(out, "Registered Dendrite MCP Tools") {
		t.Fatalf("tools: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "mcp", "list", "--json"); code != ExitOK || !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Fatalf("tools --json: exit %d\n%s", code, out)
	}
	cases := []struct {
		args []string
		code int
	}{
		{[]string{"inter", "call", "no_such_tool"}, ExitError},
		{[]string{"inter", "call", "x", "{not json"}, ExitUsage},
		{[]string{"inter", "call"}, ExitUsage},
		{[]string{"inter", "tools", "--synapse", "missing"}, ExitError},
		{[]string{"inter", "serve", "--synapse=missing"}, ExitError},
	}
	for _, c := range cases {
		if out, code := cliOutput(t, c.args...); code != c.code {
			t.Errorf("%v: exit %d, want %d\n%s", c.args, code, c.code, out)
		}
	}
}

func TestInter_ServeEOF(t *testing.T) {
	cmd := newInterCmd()
	cmd.SetIn(strings.NewReader(""))
	var buf bytes.Buffer
	if err := runWith(context.Background(), &buf, cmd, "inter", "serve"); err != nil {
		t.Fatalf("serve on empty stdin: %v", err)
	}
}
