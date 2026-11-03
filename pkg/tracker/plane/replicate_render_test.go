package plane

import (
	"strings"
	"testing"

	"Dendrite/pkg/tracker"
)

func TestRenderDevStatusMarkdownShowsOpenAndDoneComplexityRows(t *testing.T) {
	done := []ticketItem{{id: "DEN-036", status: tracker.StatusDone}, {id: "DEN-037", status: tracker.StatusDone}, {id: "DEN-038", status: tracker.StatusDone}, {id: "DEN-039", status: tracker.StatusDone}}
	open := []ticketItem{{id: "DEN-036", status: tracker.StatusTodo}, {id: "DEN-037"}, {id: "DEN-038"}, {id: "DEN-039"}}
	report := &StatusReplicationReport{}

	doneMD := renderDevStatusMarkdown(done, report)
	if got := strings.Count(doneMD, "**COMPLETED [x]**"); got != 9 {
		t.Errorf("completed rows = %d, want the five fixed rows and all four tickets (9)", got)
	}
	openMD := renderDevStatusMarkdown(open, report)
	if !strings.Contains(openMD, "**CURRENT FOCUS [ ]**") || strings.Count(openMD, "**QUEUED [ ]**") != 3 {
		t.Errorf("open rows missing from:\n%s", openMD)
	}
	if strings.Contains(renderDevStatusMarkdown(nil, report), "DEN-036") {
		t.Error("tickets that are not on the board have no row")
	}
}

func TestRenderDevStatusMarkdownGroupsByMilestone(t *testing.T) {
	items := []ticketItem{
		{id: "DEN-2", num: 2, title: "second", milestone: "Milestone 2: B", status: tracker.StatusDone, priority: "High", targetPkg: "pkg/x"},
		{id: "DEN-1", num: 1, title: "first", milestone: "Milestone 2: B"},
		{id: "DEN-9", num: 9, title: "loose"},
		{id: "A", num: 3, title: "same number a", milestone: "Milestone 1: A"},
		{id: "B", num: 3, title: "same number b", milestone: "Milestone 1: A"},
		{id: "Y-1", num: 1, title: "named", milestone: "Alpha"},
	}
	md := renderDevStatusMarkdown(items, &StatusReplicationReport{})

	order := []string{"### Milestone 1: A", "### Milestone 2: B", "### Alpha", "### General"}
	last := -1
	for _, want := range order {
		i := strings.Index(md, want)
		if i < last {
			t.Fatalf("%q is missing or out of order in:\n%s", want, md)
		}
		last = i
	}
	for _, want := range []string{
		"- [x] **DEN-2**: second (`pkg/x`) [State: Done, Priority: High]",
		"- [ ] **DEN-9**: loose [State:",
		"Priority: Medium]",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Index(md, "**DEN-1**") > strings.Index(md, "**DEN-2**") {
		t.Error("tickets are ordered by number")
	}
	if strings.Index(md, "**A**") > strings.Index(md, "**B**") {
		t.Error("equal numbers are ordered by id")
	}
	if strings.Contains(md, "Other Plane Projects") {
		t.Error("a status file has no section for other projects")
	}
}

func TestRenderDevStatusMarkdownNamesItsOwnProject(t *testing.T) {
	peer := renderDevStatusMarkdown([]ticketItem{{id: "DCORE-1", num: 1, title: "sidecar", milestone: "M"}},
		&StatusReplicationReport{ProjectIdentifier: "DCORE"})
	if !strings.HasPrefix(peer, "# DCORE Project Development Status & Roadmap") || !strings.Contains(peer, "**DCORE**, managed") {
		t.Errorf("header does not name the project:\n%s", peer)
	}
	for _, unwanted := range []string{"Complexity Analysis", "Active Milestone", "Dendrite"} {
		if strings.Contains(peer, unwanted) {
			t.Errorf("a peer file must not carry the Dendrite-only %q", unwanted)
		}
	}
	den := renderDevStatusMarkdown(nil, &StatusReplicationReport{ProjectIdentifier: "DEN", ActiveMilestone: "M17"})
	if !strings.Contains(den, "# Dendrite Project") || !strings.Contains(den, "Complexity Analysis") || !strings.Contains(den, "Active Milestone**: M17") {
		t.Errorf("the Dendrite file keeps its header, table and milestone:\n%s", den)
	}
}

func TestTicketDescriptionHTMLNeverComesOutEmpty(t *testing.T) {
	got := buildTicketDescriptionHTML(ParsedDevTicket{ID: "DEN-1", Title: "Only a title"})
	if !strings.Contains(got, "Context and implementation requirements for DEN-1: Only a title") {
		t.Errorf("got %s", got)
	}
}

func TestExtractTicketNumber(t *testing.T) {
	for id, want := range map[string]int{"DEN-042": 42, "X-7-8": 7, "NONE": 0, "": 0} {
		if got := extractTicketNumber(id); got != want {
			t.Errorf("extractTicketNumber(%q) = %d, want %d", id, got, want)
		}
	}
}
