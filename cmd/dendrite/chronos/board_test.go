package chronos

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/cmd/dendrite/internal/planecli"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func chronosOutput(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	err := runCmd(context.Background(), &buf, append([]string{"chronos"}, args...)...)
	return buf.String(), err
}

func TestChronos_ReadCommands(t *testing.T) {
	clitest.ServeBoard(t)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"next"}, "Selected Ticket:    DEN-100"},
		{[]string{"next", "--json"}, `"selected_ticket"`},
		{[]string{"next", "--milestone", "Second"}, "DEN-101"},
		{[]string{"issues"}, "#101: [DEN-101] Second [2026-10-01 -> 2026-10-05]"},
		{[]string{"list", "--json"}, `"uuid-100"`},
		{[]string{"modules"}, "M1 (Status: in-progress, ID: mod-1)"},
		{[]string{"modules", "--json"}, `"mod-1"`},
		{[]string{"pages"}, "Roadmap (ID: page-1"},
		{[]string{"pages", "--json"}, `"page-1"`},
	}
	for _, c := range cases {
		out, err := chronosOutput(t, c.args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", c.args, err, out)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%v: want %q in\n%s", c.args, c.want, out)
		}
	}
}

func TestChronos_SmartSelectPicksBlockedTicketWarning(t *testing.T) {
	clitest.ServeBoard(t)
	out, err := chronosOutput(t, "next", "--ticket", "DEN-101")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "BLOCKED by unresolved prerequisites: DEN-100") {
		t.Errorf("want blocker warning, got\n%s", out)
	}
}

func TestChronos_LinkAndRelate(t *testing.T) {
	b := clitest.ServeBoard(t)

	expectChronos(t, cli.ExitOK, "link", "DEN-101", "relates_to", "DEN-100")
	expectChronos(t, cli.ExitUsage, "link", "DEN-101", "bogus", "DEN-100")
	expectChronos(t, cli.ExitUsage, "link", "DEN-101", "relates_to")
	expectChronos(t, cli.ExitError, "link", "DEN-999", "relates_to", "DEN-100")
	expectChronos(t, cli.ExitError, "link", "DEN-101", "relates_to", "DEN-999")

	expectChronos(t, cli.ExitUsage, "relate", "--priority", "high")
	expectChronos(t, cli.ExitError, "relate", "--ticket", "DEN-999")
	out, err := chronosOutput(t, "relate", "--ticket", "DEN-101", "--priority", "HIGH", "--start", "2026-10-01", "--target", "2026-10-09",
		"--blocked-by", "DEN-100", "--blocking", "100", "--relates-to", "DEN-100,DEN-999,", "--reason", "shared parser")
	if err == nil || !strings.Contains(err.Error(), "relates_to DEN-999") {
		t.Fatalf("want an error for the unknown DEN-999, got %v", err)
	}
	for _, want := range []string{"Priority: HIGH", "DEN-101 blocked_by DEN-100", "DEN-101 blocking 100", "DEN-101 relates_to DEN-100", "Stored context: shared parser"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}

	b.Mu.Lock()
	defer b.Mu.Unlock()
	if got := strings.Join(b.Relations, ","); got != "relates_to,blocked_by,blocking,relates_to" {
		t.Errorf("relations sent: %s", got)
	}
	if b.Patches != 1 {
		t.Errorf("want one metadata PATCH, got %d", b.Patches)
	}
}

func TestChronos_TicketListAndSet(t *testing.T) {
	clitest.ServeBoard(t)
	dir := t.TempDir()
	status := filepath.Join(dir, "DEV_STATUS.md")
	if err := os.WriteFile(status, []byte("# Status\n\n## M1\n\n- [ ] **DEN-100**: First [State: To Do, Priority: High]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := chronosOutput(t, "ticket", "list", "--file", status)
	if err != nil || !strings.Contains(out, "DEN-100") {
		t.Fatalf("ticket list: %v\n%s", err, out)
	}
	if out, err := chronosOutput(t, "ticket", "ls", "-f", status, "--json"); err != nil || !strings.Contains(out, `"DEN-100"`) {
		t.Fatalf("ticket list --json: %v\n%s", err, out)
	}
	// A missing status file falls back to the Plane board.
	if out, err := chronosOutput(t, "ticket", "list", "--file", filepath.Join(dir, "missing.md")); err != nil || !strings.Contains(out, "from Plane (2)") {
		t.Fatalf("Plane fallback: %v\n%s", err, out)
	}

	expectChronos(t, cli.ExitUsage, "ticket", "set", "DEN-100", "bogus-status")
	expectChronos(t, cli.ExitUsage, "ticket", "set", "DEN-100")
	out, err = chronosOutput(t, "ticket", "set", "DEN-100", "todo", "--file", status)
	if err != nil {
		t.Fatalf("ticket set: %v\n%s", err, out)
	}
	if !strings.Contains(out, "reflected in Plane service") {
		t.Errorf("want Plane sync noted, got %q", out)
	}
}

func TestChronos_TicketWithoutPlane(t *testing.T) {
	prev := planecli.NewPlaneClient
	planecli.NewPlaneClient = func() (*plane.Client, error) { return nil, errors.New("no Plane configured") }
	t.Cleanup(func() { planecli.NewPlaneClient = prev })
	dir := t.TempDir()

	expectChronos(t, cli.ExitError, "ticket", "list", "--file", filepath.Join(dir, "missing.md"))
	expectChronos(t, cli.ExitError, "ticket", "set", "DEN-1", "done", "--file", filepath.Join(dir, "missing.md"))
	expectChronos(t, cli.ExitError, "issues")
	if err := planecli.RunPlaneDocSync(context.Background(), "push", dir, io.Discard); err == nil {
		t.Fatal("doc sync without Plane should fail")
	}
}

func TestChronos_ServerFailures(t *testing.T) {
	b := clitest.ServeBoard(t)
	b.Fail = true
	dir := t.TempDir()
	status := filepath.Join(dir, "DEV_STATUS.md")
	_ = os.WriteFile(status, []byte("- [ ] **DEN-1**: x\n"), 0o644)

	for _, args := range [][]string{
		{"next"}, {"issues"}, {"modules"}, {"pages"}, {"gantt-sync"},
		{"replicate", "--dry-run"}, {"push", "--file", status}, {"push", "--file", filepath.Join(dir, "missing.md")},
		{"ticket", "set", "DEN-1", "todo", "--file", status},
	} {
		expectChronos(t, cli.ExitError, args...)
	}
}

func TestChronos_ReplicateAndPush(t *testing.T) {
	clitest.ServeBoard(t)
	dir := t.TempDir()
	status := filepath.Join(dir, "DEV_STATUS.md")

	if out, err := chronosOutput(t, "replicate", "--dry-run", "--file", status); err != nil || !strings.Contains(out, "Dry run mode") {
		t.Fatalf("replicate --dry-run: %v\n%s", err, out)
	}
	if _, err := os.Stat(status); !os.IsNotExist(err) {
		t.Fatal("dry run must not write the status file")
	}
	if out, err := chronosOutput(t, "pull", "-f", status); err != nil || !strings.Contains(out, "Successfully replicated") {
		t.Fatalf("replicate: %v\n%s", err, out)
	}
	if out, err := chronosOutput(t, "push", "--file", status); err != nil || !strings.Contains(out, "Completed moving DEV_STATUS") {
		t.Fatalf("push: %v\n%s", err, out)
	}
	if out, err := chronosOutput(t, "gantt-sync"); err != nil || !strings.Contains(out, "Live Gantt View URL") {
		t.Fatalf("gantt-sync: %v\n%s", err, out)
	}
}

func TestRunPlaneDocSync(t *testing.T) {
	clitest.ServeBoard(t)
	dir := t.TempDir()
	if err := planecli.RunPlaneDocSync(context.Background(), "sideways", dir, io.Discard); !cli.IsUsageError(err) {
		t.Fatalf("bad direction should be a usage error, got %v", err)
	}
	for _, d := range []string{"push", "pull", "sync"} {
		var buf bytes.Buffer
		_ = planecli.RunPlaneDocSync(context.Background(), d, dir, &buf)
		if !strings.Contains(buf.String(), "Synchronizing project documentation") {
			t.Errorf("%s: no progress output: %q", d, buf.String())
		}
	}
}

func TestPlaneStatusFor(t *testing.T) {
	cases := map[string]tracker.Status{
		"completed/Shipped":     tracker.StatusDone,
		"started/To Verify":     tracker.StatusInReview,
		"started/In Progress":   tracker.StatusInProgress,
		"unstarted/Todo":        tracker.StatusTodo,
		"backlog/Ideas":         tracker.StatusBacklog,
		"cancelled/Cancelled":   tracker.StatusCancelled,
		"started/Blocked":       tracker.StatusBlocked,
		"unstarted/Active work": tracker.StatusInProgress,
	}
	for in, want := range cases {
		g, n, _ := strings.Cut(in, "/")
		if got := planeStatusFor(plane.PlaneState{Group: g, Name: n}); got != want {
			t.Errorf("%s: got %s, want %s", in, got, want)
		}
	}
}

// Runners over the commands this package builds (the full tree lives in package main).
var runCmd = clitest.Runner(NewChronosCmd)

func TestMain(m *testing.M) { clitest.Main(m) }
