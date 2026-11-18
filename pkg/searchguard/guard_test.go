package searchguard

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newTestGuard(t *testing.T) (*Guard, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	g := New(t.TempDir())
	g.Now = func() time.Time { return now }
	g.Getenv = func(string) string { return "" }
	return g, &now
}

func bash(session, cmd string) HookInput {
	return HookInput{SessionID: session, Cwd: "/repo", ToolName: "Bash", ToolInput: ToolInput{Command: cmd}}
}

func tick(now *time.Time) { *now = now.Add(time.Second) }

func TestRetryIsAllowedOnlyAfterALookup(t *testing.T) {
	g, now := newTestGuard(t)
	d := g.Check(bash("s1", `rg "Foo" pkg/`))
	if !d.Block || !strings.Contains(d.Message, "dendrite rag search") || !strings.Contains(d.Message, "run this same command again") {
		t.Fatalf("first attempt = %+v, want a block that names the dendrite equivalents", d)
	}
	// The same command with no lookup in between is denied, with a message that says why.
	tick(now)
	if d := g.Check(bash("s1", `rg "Foo"   pkg/`)); !d.Block || d.Message != DeniedHint {
		t.Errorf("retry without a lookup = %+v, want the denied hint", d)
	}
	// Another session's lookup does not count.
	tick(now)
	g.Check(bash("s2", `dendrite rag search "x"`))
	if d := g.Check(bash("s1", `rg "Foo" pkg/`)); !d.Block {
		t.Error("a lookup by another session earned this session's retry")
	}
	// A lookup by this session does (whitespace aside).
	tick(now)
	g.Check(bash("s1", `dendrite rag search "foo"`))
	tick(now)
	if d := g.Check(bash("s1", `rg "Foo"   pkg/`)); d.Block {
		t.Error("the retry after a lookup was blocked")
	}
	// A different command is a new search: stopped once as well.
	if d := g.Check(bash("s1", `rg "Bar" pkg/`)); !d.Block {
		t.Error("a different search was not warned about")
	}
}

func TestRetryAllowanceEndsWithTheWindow(t *testing.T) {
	g, now := newTestGuard(t)
	g.Check(bash("s1", `rg "Foo" pkg/`))
	tick(now)
	g.Check(bash("s1", `dendrite ask "q"`))
	*now = now.Add(RetryWindow + time.Minute)
	if d := g.Check(bash("s1", `rg "Foo" pkg/`)); !d.Block || d.Message != Hint {
		t.Error("an allowance outlived the window")
	}
}

func TestOnlyLookupsEarnTheRetry(t *testing.T) {
	cases := []struct {
		name  string
		input HookInput
		earns bool
	}{
		{"rag search", bash("s", `dendrite rag search "x"`), true},
		{"ask", bash("s", `dendrite ask "x"`), true},
		{"graph", bash("s", `dendrite graph explain Foo`), true},
		{"brief", bash("s", `dendrite brief "x"`), true},
		{"mcp retrieve", HookInput{SessionID: "s", ToolName: "mcp__dendrite__dendrite_rag_retrieve"}, true},
		{"mcp graph", HookInput{SessionID: "s", ToolName: "mcp__dendrite__dendrite_graph_explain"}, true},
		{"chronos", bash("s", `dendrite chronos next`), false},
		{"session", bash("s", `dendrite session resume`), false},
		{"mcp ticket", HookInput{SessionID: "s", ToolName: "mcp__dendrite__dendrite_ticket_list"}, false},
		{"ls", bash("s", `ls`), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, now := newTestGuard(t)
			g.Check(bash("s", `rg Foo .`))
			tick(now)
			g.Check(c.input)
			tick(now)
			if blocked := g.Check(bash("s", `rg Foo .`)).Block; blocked == c.earns {
				t.Errorf("retry blocked = %v after %s, want %v", blocked, c.name, !c.earns)
			}
		})
	}
}

func TestSearchingNamedFilesIsNeverStopped(t *testing.T) {
	g, _ := newTestGuard(t)
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/a.go", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir+"/sub", 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		in      HookInput
		blocked bool
	}{
		{"rg one file", HookInput{SessionID: "s", Cwd: dir, ToolName: "Bash", ToolInput: ToolInput{Command: `rg -n Foo a.go`}}, false},
		{"absolute file", HookInput{SessionID: "s", Cwd: "/elsewhere", ToolName: "Bash", ToolInput: ToolInput{Command: `rg Foo ` + dir + `/a.go`}}, false},
		{"grep -r one file", HookInput{SessionID: "s", Cwd: dir, ToolName: "Bash", ToolInput: ToolInput{Command: `grep -rn Foo a.go`}}, false},
		{"Grep tool file", HookInput{SessionID: "s", Cwd: dir, ToolName: "Grep", ToolInput: ToolInput{Pattern: "Foo", Path: "a.go"}}, false},
		{"-e pattern then file", HookInput{SessionID: "s", Cwd: dir, ToolName: "Bash", ToolInput: ToolInput{Command: `rg -e Foo a.go`}}, false},
		{"directory", HookInput{SessionID: "s", Cwd: dir, ToolName: "Bash", ToolInput: ToolInput{Command: `rg Foo sub`}}, true},
		{"no path", HookInput{SessionID: "s", Cwd: dir, ToolName: "Bash", ToolInput: ToolInput{Command: `rg Foo`}}, true},
		{"missing file", HookInput{SessionID: "s", Cwd: dir, ToolName: "Bash", ToolInput: ToolInput{Command: `rg Foo nope.go`}}, true},
		{"file and directory", HookInput{SessionID: "s", Cwd: dir, ToolName: "Bash", ToolInput: ToolInput{Command: `rg Foo a.go sub`}}, true},
		{"Grep tool no path", HookInput{SessionID: "s", Cwd: dir, ToolName: "Grep", ToolInput: ToolInput{Pattern: "Foo"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := g.Check(c.in).Block; got != c.blocked {
				t.Errorf("blocked = %v, want %v", got, c.blocked)
			}
		})
	}
}

func TestLegacyStateFileIsStillRead(t *testing.T) {
	g, now := newTestGuard(t)
	// The earlier format: a flat map of command hash to Unix seconds.
	if err := os.WriteFile(g.statePath(), []byte(`{"abc":`+itoa(now.Unix())+`}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st := g.loadState(*now)
	if _, ok := st.Blocked["abc"]; !ok {
		t.Errorf("legacy entry not read: %+v", st)
	}
	*now = now.Add(RetryWindow + time.Minute)
	if st := g.loadState(*now); len(st.Blocked) != 0 {
		t.Errorf("legacy entry outlived the window: %+v", st)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestBypassAndDendrite(t *testing.T) {
	g, _ := newTestGuard(t)
	g.Getenv = func(k string) string {
		if k == AllowEnv {
			return "1"
		}
		return ""
	}
	if g.Check(bash("s", `rg Foo`)).Block {
		t.Error("DENDRITE_ALLOW_RG=1 did not bypass")
	}
	g.Getenv = func(string) string { return "" }
	if g.Check(bash("s", `DENDRITE_ALLOW_RG=1 rg Foo`)).Block {
		t.Error("an inline DENDRITE_ALLOW_RG=1 did not bypass")
	}
	if g.Check(bash("s", `dendrite rag search "x"`)).Block {
		t.Error("a dendrite call was blocked")
	}
	if g.Check(HookInput{SessionID: "s", ToolName: "mcp__dendrite__dendrite_rag_retrieve"}).Block {
		t.Error("a dendrite MCP call was blocked")
	}
	if g.Check(bash("s", `ls`)).Block || g.Check(HookInput{ToolName: "Read"}).Block {
		t.Error("an unrelated tool was blocked")
	}
}

func TestGrepToolIsGuardedLikeBash(t *testing.T) {
	g, _ := newTestGuard(t)
	in := HookInput{SessionID: "s", ToolName: "Grep", ToolInput: ToolInput{Pattern: "Foo", Path: "pkg"}}
	if !g.Check(in).Block {
		t.Fatal("the built-in Grep tool was not warned about")
	}
	if !g.Check(in).Block {
		t.Error("the repeated Grep call went through without a lookup")
	}
}

func TestSummaryCountsWhatRanAndWhatWasStopped(t *testing.T) {
	g, now := newTestGuard(t)
	g.Check(bash("s", `rg A`)) // blocked
	tick(now)
	g.Check(bash("s", `rg A`))                                       // denied: no lookup yet
	g.Check(bash("s", `grep -r B .`))                                // blocked
	g.Check(bash("s", `dendrite rag search "x"`))                    // dendrite, earns
	g.Check(bash("s", `dendrite graph explain Foo`))                 // dendrite
	g.Check(HookInput{SessionID: "s", ToolName: "mcp__dendrite__x"}) // dendrite mcp
	g.Check(HookInput{SessionID: "s", ToolName: "Grep", ToolInput: ToolInput{Pattern: "q"}})
	tick(now)
	g.Check(bash("s", `rg A`)) // runs: a lookup came in between
	f, err := os.Open(g.LogPath())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := Summarize(f, now.Add(-time.Hour))
	if s.Dendrite != 3 {
		t.Errorf("dendrite = %d, want 3", s.Dendrite)
	}
	if s.RawSearches[KindRg] != 1 || s.Blocked[KindRg] != 1 || s.Blocked[KindGrep] != 1 || s.Blocked[KindGrepTool] != 1 || s.Denied != 1 {
		t.Errorf("summary = %+v", s)
	}
	if want := 3.0 / 4.0; s.Share != want {
		t.Errorf("share = %v, want %v", s.Share, want)
	}
}

func TestSummaryIgnoresEventsBeforeTheWindow(t *testing.T) {
	g, now := newTestGuard(t)
	g.Check(bash("s", `dendrite rag search "x"`))
	f, err := os.Open(g.LogPath())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if later := Summarize(f, now.Add(time.Hour)); later.Dendrite != 0 || later.Share != 1 {
		t.Errorf("late window = %+v", later)
	}
}
