package searchguard

import (
	"os"
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

func TestFirstSearchIsBlockedAndTheRepeatIsAllowed(t *testing.T) {
	g, now := newTestGuard(t)
	d := g.Check(bash("s1", `rg "Foo" pkg/`))
	if !d.Block || !strings.Contains(d.Message, "dendrite rag search") || !strings.Contains(d.Message, "run this same command again") {
		t.Fatalf("first attempt = %+v, want a block that names the dendrite equivalents", d)
	}
	// The same command again is a deliberate retry.
	if d := g.Check(bash("s1", `rg "Foo"   pkg/`)); d.Block {
		t.Error("the repeated command (whitespace aside) was blocked again")
	}
	// A different command is a new search: warned once as well.
	if d := g.Check(bash("s1", `rg "Bar" pkg/`)); !d.Block {
		t.Error("a different search was not warned about")
	}
	// Another session starts over.
	if d := g.Check(bash("s2", `rg "Foo" pkg/`)); !d.Block {
		t.Error("a new session inherited another session's retry")
	}
	// The retry window ends.
	*now = now.Add(RetryWindow + time.Minute)
	if d := g.Check(bash("s1", `rg "Foo" pkg/`)); !d.Block {
		t.Error("a retry allowance outlived the window")
	}
}

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
	if g.Check(in).Block {
		t.Error("the repeated Grep call was blocked")
	}
}

func TestSummaryCountsWhatRanAndWhatWasStopped(t *testing.T) {
	g, now := newTestGuard(t)
	g.Check(bash("s", `rg A`))                                       // blocked
	g.Check(bash("s", `rg A`))                                       // runs
	g.Check(bash("s", `grep -r B .`))                                // blocked
	g.Check(bash("s", `dendrite rag search "x"`))                    // dendrite
	g.Check(bash("s", `dendrite graph explain Foo`))                 // dendrite
	g.Check(HookInput{SessionID: "s", ToolName: "mcp__dendrite__x"}) // dendrite mcp
	g.Check(HookInput{SessionID: "s", ToolName: "Grep", ToolInput: ToolInput{Pattern: "q"}})
	f, err := os.Open(g.LogPath())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := Summarize(f, now.Add(-time.Hour))
	if s.Dendrite != 3 {
		t.Errorf("dendrite = %d, want 3", s.Dendrite)
	}
	if s.RawSearches[KindRg] != 1 || s.Blocked[KindRg] != 1 || s.Blocked[KindGrep] != 1 || s.Blocked[KindGrepTool] != 1 {
		t.Errorf("summary = %+v", s)
	}
	if want := 3.0 / 4.0; s.Share != want {
		t.Errorf("share = %v, want %v", s.Share, want)
	}
	// Events before the window are ignored.
	f.Seek(0, 0)
	if later := Summarize(f, now.Add(time.Hour)); later.Dendrite != 0 || later.Share != 1 {
		t.Errorf("late window = %+v", later)
	}
}
