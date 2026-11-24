package insight

import (
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/rea"
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
)

func TestSummaryNamesWhatTheAnalysisFoundAndHowToReadMore(t *testing.T) {
	res := &analysis.Result{State: analysis.State{ID: "an-1", Binary: "/x/sample.exe", Kind: "executable", Format: "pe", Tier: "local", Stage: analysis.StageDone},
		Bundle: &rea.EvidenceBundle{TokenEstimate: 120, CrossReferences: []string{"A.dll", "B.dll"}, StringConstants: []string{"E"}}}
	text := Summary(res)
	for _, want := range []string{"REA analysis an-1: sample.exe (executable, pe)", "tier local", "~120 tokens", "dependencies: 2, exports: 1", "dendrite retro search an-1", "dendrite retro resume an-1"} {
		if !strings.Contains(text, want) {
			t.Errorf("summary lacks %q:\n%s", want, text)
		}
	}
	paused := Summary(&analysis.Result{State: analysis.State{ID: "an-2", Binary: "/win/app.exe", Stage: analysis.StageParsed}})
	if !strings.Contains(paused, "app.exe") || strings.Contains(paused, "Evidence") {
		t.Errorf("a paused analysis has no evidence yet:\n%s", paused)
	}
}

func TestPostNoteLeavesTheSummaryOnTheTicket(t *testing.T) {
	board := planetest.NewBoard(t, map[string]string{"DEN-1": "In Progress"})
	client, err := plane.NewClient(plane.Config{BaseURL: board.URL, WorkspaceSlug: planetest.BoardWorkspace, ProjectID: planetest.BoardProject, DirectToken: "k"}, board.Client())
	if err != nil {
		t.Fatal(err)
	}
	svc := planesvc.New(sidecar.DefaultLocator())
	t.Cleanup(func() { _ = svc.Close() })
	l := &ticketlock.Lifecycle{Board: ticketlock.Board{Svc: svc, Client: client}, Store: &ticketlock.Store{Dir: t.TempDir()}, Plans: ticketlock.PlanStore{Dir: t.TempDir()},
		Now: time.Now, Dir: t.TempDir(), Branch: func(string) string { return "b" }, Head: func(string) string { return "abc" }}

	res := &analysis.Result{State: analysis.State{ID: "an-1", Binary: "/x/a.exe", Format: "elf", Stage: analysis.StageDone}}
	if err := PostNote(ctx(), l, "DEN-1", res); err != nil {
		t.Fatal(err)
	}
	texts := board.Texts("DEN-1")
	if len(texts) != 1 || !strings.HasPrefix(texts[0], "[dendrite:note]") || !strings.Contains(texts[0], "REA analysis an-1") {
		t.Errorf("comments %q", texts)
	}
	board.Fail("comments/", 0)
	if err := PostNote(ctx(), l, "DEN-1", res); err == nil {
		t.Error("a ticket that cannot be commented on is an error for the caller to report")
	}
}
