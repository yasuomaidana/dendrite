package hook

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/searchguard"
	"bytes"
	"strings"
	"testing"
)

func TestPreToolHookBlocksOnceAllowsTheRepeatAndFailsOpen(t *testing.T) {
	g := searchguard.New(t.TempDir())
	event := `{"session_id":"s","cwd":"/r","tool_name":"Bash","tool_input":{"command":"rg Foo pkg/"}}`

	err := runPreTool(strings.NewReader(event), g)
	if err == nil || cli.ExitCodeFor(err) != cli.ExitUsage || !strings.Contains(err.Error(), "dendrite rag search") {
		t.Fatalf("first attempt: err = %v (exit %d), want exit 2 with the dendrite equivalents", err, cli.ExitCodeFor(err))
	}
	if err := runPreTool(strings.NewReader(event), g); err != nil {
		t.Errorf("the repeat was blocked: %v", err)
	}
	for _, bad := range []string{"", "not json", `{"tool_name":""}`, `{}`} {
		if err := runPreTool(strings.NewReader(bad), g); err != nil {
			t.Errorf("malformed input %q must be allowed, got %v", bad, err)
		}
	}
}

func TestSensoryUsageReportsWhatWasRecorded(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DENDRITE_HOME", dir)
	var out bytes.Buffer
	if err := runSensoryUsage(&out, 24*7*3600e9); err != nil || !strings.Contains(out.String(), "No tool use recorded") {
		t.Fatalf("empty: %q, %v", out.String(), err)
	}
	g := searchguard.New(searchGuardDir())
	_ = runPreTool(strings.NewReader(`{"session_id":"s","tool_name":"Bash","tool_input":{"command":"rg A"}}`), g)
	_ = runPreTool(strings.NewReader(`{"session_id":"s","tool_name":"Bash","tool_input":{"command":"dendrite rag search x"}}`), g)
	_ = runPreTool(strings.NewReader(`{"session_id":"s","tool_name":"Bash","tool_input":{"command":"dendrite graph explain Y"}}`), g)
	out.Reset()
	if err := runSensoryUsage(&out, 24*7*3600e9); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"7 days", "Dendrite calls:        2", "Raw searches that ran: 0", "First attempts stopped: 1", "100%"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("usage report lacks %q:\n%s", want, out.String())
		}
	}
}

// Runners over the commands this package builds (the full tree lives in package main).
var runCmd = clitest.Runner(NewHookCmd, NewSensoryCmd)
var cliOutput = clitest.Output(NewHookCmd, NewSensoryCmd)

func TestMain(m *testing.M) { clitest.Main(m) }
