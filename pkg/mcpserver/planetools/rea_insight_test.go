package planetools_test

import (
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/tracker/plane"
	"path/filepath"
	"strings"
	"testing"

	. "Dendrite/pkg/mcpserver"
)

// analyzedTools is a server with the REA tools and a finished analysis named pe-ins.
func analyzedTools(t *testing.T) *Server {
	t.Helper()
	s, pe := reaTools(t, nil)
	if _, err := callRea(t, s, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-ins"}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestReaReuseToolFindsWhatTheProjectHas(t *testing.T) {
	s := analyzedTools(t)
	project := t.TempDir()
	testkit.WriteFile(t, filepath.Join(project, "checksum.go"), "package main\n\nfunc ComputeChecksum() {}\n", 0o644)
	out, err := callRea(t, s, "dendrite_rea_reuse", map[string]any{"id": "pe-ins", "project": project, "k": float64(3)})
	if err != nil || !strings.Contains(out, "checksum.go") || !strings.Contains(out, `"source": "project"`) {
		t.Errorf("reuse: %v\n%s", err, out)
	}
}

func TestReaRankToolOrdersTheDocuments(t *testing.T) {
	out, err := callRea(t, analyzedTools(t), "dendrite_rea_rank", map[string]any{"id": "pe-ins"})
	if err != nil || !strings.Contains(out, `"doc": "overview.md"`) || !strings.Contains(out, "libraries/kernel32.dll.md") {
		t.Errorf("rank: %v\n%s", err, out)
	}
}

func TestReaADRToolDraftsAProposedRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "decisions")
	out, err := callRea(t, analyzedTools(t), "dendrite_rea_adr", map[string]any{"id": "pe-ins", "output": dir, "title": "Recreate it"})
	if err != nil || !strings.Contains(out, "ADR-001.md") || !strings.Contains(out, `"status": "proposed"`) {
		t.Errorf("adr: %v\n%s", err, out)
	}
}

func TestReaInsightToolsReportWhatTheyCannotDo(t *testing.T) {
	s, _ := reaTools(t, nil)
	for name, tc := range map[string]struct {
		tool string
		args map[string]any
		want string
	}{
		"reuse needs an id":      {"dendrite_rea_reuse", map[string]any{}, "id is required"},
		"rank needs an id":       {"dendrite_rea_rank", map[string]any{}, "id is required"},
		"adr needs an id":        {"dendrite_rea_adr", map[string]any{}, "id is required"},
		"reuse of an unknown id": {"dendrite_rea_reuse", map[string]any{"id": "pe-none"}, "no such analysis"},
		"rank of an unknown id":  {"dendrite_rea_rank", map[string]any{"id": "pe-none"}, "no such analysis"},
		"adr of an unknown id":   {"dendrite_rea_adr", map[string]any{"id": "pe-none"}, "no such analysis"},
	} {
		expectToolError(t, s, name, tc.tool, tc.args, tc.want)
	}
}

// ticketTools is a server whose tools reach a Plane test board holding DEN-1.
func ticketTools(t *testing.T) (*Server, *planetest.Board, string) {
	t.Helper()
	board := planetest.NewBoard(t, map[string]string{"DEN-1": "In Progress"})
	client, err := plane.NewClient(plane.Config{BaseURL: board.URL, WorkspaceSlug: planetest.BoardWorkspace, ProjectID: planetest.BoardProject, DirectToken: "k"}, board.Client())
	if err != nil {
		t.Fatal(err)
	}
	s, pe := reaToolsWith(t, Dependencies{PlaneClient: client}, nil)
	return s, board, pe
}

func TestReaAnalyzeLeavesANoteOnTheTicket(t *testing.T) {
	s, board, pe := ticketTools(t)
	out, err := callRea(t, s, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-n1", "ticket": "DEN-1"})
	if err != nil || !strings.Contains(out, `"ticket_note": "posted"`) || !strings.Contains(out, `"stage": "done"`) {
		t.Fatalf("a note on the ticket: %v\n%s", err, out)
	}
	if texts := board.Texts("DEN-1"); len(texts) != 1 || !strings.Contains(texts[0], "REA analysis pe-n1") {
		t.Errorf("comments %q", texts)
	}
}

func TestReaAnalyzeReportsATicketNoteThatCannotBePosted(t *testing.T) {
	s, board, pe := ticketTools(t)
	board.Fail("comments/", 0)
	out, err := callRea(t, s, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-n2", "ticket": "DEN-1"})
	if err != nil || !strings.Contains(out, `"ticket_note": "not posted:`) {
		t.Errorf("a ticket that cannot be commented on is reported in the result: %v\n%s", err, out)
	}
}

func TestReaAnalyzeWithoutAPlaneClientSaysWhyTheNoteWasNotPosted(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "")
	t.Setenv("PLANE_BASE_URL", "")
	bare, pe := reaTools(t, nil)
	out, err := callRea(t, bare, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-n3", "ticket": "DEN-1"})
	if err != nil || !strings.Contains(out, "not posted: plane client error") {
		t.Errorf("without a Plane client: %v\n%s", err, out)
	}
}
