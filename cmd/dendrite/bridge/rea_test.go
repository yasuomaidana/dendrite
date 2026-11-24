package bridge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/directive"
	"Dendrite/pkg/rea/ownership"
	"Dendrite/pkg/ticketlock"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetro(t *testing.T) {
	if out, code := cliOutput(t, "retro", "list"); code != cli.ExitOK || !strings.Contains(out, "Showcase Targets (5)") {
		t.Fatalf("list: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "rea", "targets", "--json"); code != cli.ExitOK || !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Fatalf("list --json: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "retro", "showcase", "aegis"); code != cli.ExitOK || !strings.Contains(out, "SCORECARD: PASSED") {
		t.Fatalf("showcase text: exit %d\n%s", code, out)
	}
	file := filepath.Join(t.TempDir(), "card.md")
	if out, code := cliOutput(t, "retro", "showcase", "dxball", "--format", "md", "-o", file); code != cli.ExitOK || !strings.Contains(out, "Scorecard written to") {
		t.Fatalf("showcase markdown file: exit %d\n%s", code, out)
	}
	if data, err := os.ReadFile(file); err != nil || !strings.Contains(string(data), "# Open-Source REA Showcase") {
		t.Fatalf("markdown file: %v", err)
	}
	for _, args := range [][]string{
		{"retro", "showcase", "aegis", "--format", "json"},
		{"retro", "showcase", "a", "b"},
	} {
		if _, code := cliOutput(t, args...); code != cli.ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, cli.ExitUsage)
		}
	}
	if _, code := cliOutput(t, "retro", "showcase", "no-such-target"); code != cli.ExitError {
		t.Errorf("unknown target: exit %d", code)
	}
	if _, code := cliOutput(t, "retro", "showcase", "ctf", "-o", filepath.Join(t.TempDir(), "missing", "x.md")); code != cli.ExitError {
		t.Errorf("unwritable output: exit %d", code)
	}
}

// handoffRun is what one handoff command did to its ports.
type handoffRun struct {
	comments, labels, released int
	spec                       agentjobs.SubmitSpec
}

// useHandoffPorts points the command at ports over issue that record into the returned run.
func useHandoffPorts(t *testing.T, issue ownership.Issue) *handoffRun {
	t.Helper()
	clitest.ServeBoard(t)
	r := &handoffRun{}
	old := retroPorts
	t.Cleanup(func() { retroPorts = old })
	retroPorts = func(*ticketlock.Lifecycle) ownership.Ports {
		return ownership.Ports{
			Issue:         func(context.Context, string) (ownership.Issue, error) { return issue, nil },
			Comment:       func(context.Context, string, string, string) error { r.comments++; return nil },
			AddLabel:      func(context.Context, string, string) error { r.labels++; return nil },
			ReleaseClaims: func(string) error { r.released++; return nil },
			Submit:        func(s agentjobs.SubmitSpec) (string, error) { r.spec = s; return "job-9", nil },
		}
	}
	return r
}

func TestRetroHandoffHandsARefusedTicketToAGY(t *testing.T) {
	r := useHandoffPorts(t, ownership.Issue{Ticket: "DEN-1062", Title: "[DEN-1062] [REA] analyze", Estimate: 4})
	out, code := cliOutput(t, "retro", "handoff", "DEN-1062", "--refused", "--reason", "declined the binary")
	if code != cli.ExitOK || !strings.Contains(out, "DEN-1062 handed to AGY (job job-9)") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if r.comments != 1 || r.labels != 1 || r.released != 1 || r.spec.Agent != "agy" || !strings.Contains(r.spec.Prompt, "declined the binary") {
		t.Errorf("effects %+v", r)
	}
	if out, code := cliOutput(t, "retro", "handoff", "DEN-1062", "--refused", "--json"); code != cli.ExitOK || !strings.Contains(out, `"job_id": "job-9"`) {
		t.Errorf("json: exit %d\n%s", code, out)
	}
}

func TestRetroHandoffNeedsRefusedAndAnREATicket(t *testing.T) {
	useHandoffPorts(t, ownership.Issue{Ticket: "DEN-5", Title: "[DEN-5] Fix the parser"})
	if _, code := cliOutput(t, "retro", "handoff", "DEN-5"); code != cli.ExitUsage {
		t.Errorf("without --refused: exit %d, want %d", code, cli.ExitUsage)
	}
	if _, code := cliOutput(t, "retro", "handoff", "--refused"); code != cli.ExitUsage {
		t.Errorf("without a ticket: exit %d, want %d", code, cli.ExitUsage)
	}
	if _, code := cliOutput(t, "retro", "handoff", "DEN-5", "--refused"); code != cli.ExitError {
		t.Errorf("not an REA ticket: exit %d, want %d", code, cli.ExitError)
	}
}

func TestRetroHandoffOfATicketAGYOwnsDoesNothing(t *testing.T) {
	r := useHandoffPorts(t, ownership.Issue{Ticket: "DEN-1062", Title: "[DEN-1062] [REA] x", Labels: []string{ownership.OwnerLabel}})
	out, code := cliOutput(t, "retro", "handoff", "DEN-1062", "--refused")
	if code != cli.ExitOK || !strings.Contains(out, "already owned by AGY") || r.comments+r.labels+r.released != 0 {
		t.Errorf("exit %d, effects %+v\n%s", code, r, out)
	}
}

func TestRetroHandoffReadsTheDirectivesOfTheWorkingCopy(t *testing.T) {
	useHandoffPorts(t, ownership.Issue{Ticket: "DEN-1062", Title: "[DEN-1062] [REA] x"})
	dir := t.TempDir() // not a repository: its own directives apply
	file := filepath.Join(dir, filepath.FromSlash(directive.Path))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(file, []byte("schema = 1\n[rea]\nrefusal_owner = \"none\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := cliOutput(t, "retro", "handoff", "DEN-1062", "--refused", "--worktree", dir); code != cli.ExitError {
		t.Errorf("rea.refusal_owner = none keeps the ticket with Claude: exit %d", code)
	}

	if err := os.WriteFile(file, []byte("not = [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, code := cliOutput(t, "retro", "handoff", "DEN-1062", "--refused", "--worktree", dir); code != cli.ExitError {
		t.Errorf("unreadable directives: exit %d", code)
	}
}
