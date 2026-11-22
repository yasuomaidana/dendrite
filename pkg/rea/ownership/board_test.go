package ownership

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/agentjobs"
	"Dendrite/pkg/directive"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
)

// reaBoard is a Plane board with one REA ticket and a lifecycle on it.
type reaBoard struct {
	*ticketlock.Lifecycle
	board *planetest.Board
}

func newReaBoard(t *testing.T, title string) *reaBoard {
	t.Helper()
	board := planetest.NewBoard(t, map[string]string{"DEN-1062": "In Progress"})
	board.SetTitle("DEN-1062", title)
	client, err := plane.NewClient(plane.Config{BaseURL: board.URL, WorkspaceSlug: planetest.BoardWorkspace, ProjectID: planetest.BoardProject, DirectToken: "k"}, board.Client())
	if err != nil {
		t.Fatal(err)
	}
	svc := planesvc.New(sidecar.DefaultLocator())
	t.Cleanup(func() { _ = svc.Close() })
	l := &ticketlock.Lifecycle{
		Board: ticketlock.Board{Svc: svc, Client: client}, Store: &ticketlock.Store{Dir: t.TempDir()},
		Plans: ticketlock.PlanStore{Dir: t.TempDir()}, Now: time.Now, Dir: t.TempDir(),
		Branch: func(string) string { return "DEND-501-x/DEN-1062-y" }, Head: func(string) string { return "abc1234" },
	}
	return &reaBoard{Lifecycle: l, board: board}
}

func TestBoardPortsTransferARefusedTicketOnARealBoard(t *testing.T) {
	b := newReaBoard(t, "[DEN-1062] [REA] retro analyze")
	b.board.SetDescription("DEN-1062", "<p>Parse PE64 headers.</p>")
	claudeBranch := "DEND-501-x/DEN-1062-y"
	if err := b.Store.WriteClaim(ticketlock.Claim{Ticket: "DEN-1062", Agent: "claude", Branch: claudeBranch, SessionID: "s-1"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Store.WriteClaim(ticketlock.Claim{Ticket: "DEN-1062", Agent: "agy", Branch: "agy-branch", SessionID: "s-2"}); err != nil {
		t.Fatal(err)
	}

	ports := BoardPorts(b.Lifecycle)
	var spec agentjobs.SubmitSpec
	ports.Submit = func(s agentjobs.SubmitSpec) (string, error) { spec = s; return "job-7", nil }

	res, err := Transfer(context.Background(), ports, directive.Defaults().Rea, "DEN-1062", Refusal{Reason: "declined", Excerpt: "I can't help with that."})
	if err != nil || res.JobID != "job-7" || res.Ticket != "DEN-1062" {
		t.Fatalf("transfer: %+v, %v", res, err)
	}
	if got := b.board.LabelNames("DEN-1062"); !reflect.DeepEqual(got, []string{OwnerLabel}) {
		t.Errorf("labels %v", got)
	}
	texts := b.board.Texts("DEN-1062")
	if len(texts) != 1 || !strings.HasPrefix(texts[0], "[dendrite:decision]") || !strings.Contains(texts[0], "AGY takes ownership") {
		t.Errorf("comments %q", texts)
	}
	if _, err := b.Store.ClaimFor(claudeBranch); !errors.Is(err, ticketlock.ErrNoClaim) {
		t.Errorf("the Claude claim must be released: %v", err)
	}
	if _, err := b.Store.ClaimFor("agy-branch"); err != nil {
		t.Errorf("the AGY claim stays: %v", err)
	}
	if spec.Ticket != "DEN-1062" || !strings.Contains(spec.Prompt, "Parse PE64 headers.") {
		t.Errorf("job spec %+v", spec)
	}

	again, err := Transfer(context.Background(), ports, directive.Defaults().Rea, "DEN-1062", Refusal{})
	if err != nil || !again.AlreadyOwned {
		t.Errorf("a second transfer changes nothing: %+v, %v", again, err)
	}
}

func TestBoardPortsReadEstimateAndRefuseNonREATickets(t *testing.T) {
	b := newReaBoard(t, "[DEN-1062] Fix the parser")
	ports := BoardPorts(b.Lifecycle)
	issue, err := ports.Issue(context.Background(), "DEN-1062")
	if err != nil || issue.Ticket != "DEN-1062" || issue.Estimate != 0 {
		t.Fatalf("issue %+v, %v", issue, err)
	}
	if _, err := Transfer(context.Background(), ports, directive.Defaults().Rea, "DEN-1062", Refusal{}); !errors.Is(err, ErrNotREA) {
		t.Errorf("not an REA ticket: %v", err)
	}
	if _, err := ports.Issue(context.Background(), "DEN-9999"); err == nil {
		t.Error("an unknown ticket is an error")
	}
}

func TestBoardPortsReportPlaneAndStoreFailures(t *testing.T) {
	b := newReaBoard(t, "[DEN-1062] [REA] x")
	ports := BoardPorts(b.Lifecycle)
	b.board.Fail("labels/", 0)
	if _, err := ports.Issue(context.Background(), "DEN-1062"); err == nil {
		t.Error("labels that cannot be read fail the issue lookup")
	}
	if err := ports.AddLabel(context.Background(), "DEN-1062", OwnerLabel); err == nil {
		t.Error("a label that cannot be added is an error")
	}

	claims := filepath.Join(b.Store.Dir, "claims")
	if err := b.Store.WriteClaim(ticketlock.Claim{Ticket: "DEN-1062", Agent: "claude", Branch: "b", SessionID: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(claims, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(claims, 0o755) })
	canWrite := os.WriteFile(filepath.Join(claims, "probe"), nil, 0o644) == nil // root ignores permissions
	if err := ports.ReleaseClaims("DEN-1062"); canWrite == (err != nil) {
		t.Errorf("writable = %v but release err = %v", canWrite, err)
	}
}

func TestPointsIsZeroWithoutAnEstimate(t *testing.T) {
	five := 5
	if points(nil) != 0 || points(&five) != 5 {
		t.Error("an estimate is its points, none is 0")
	}
}
