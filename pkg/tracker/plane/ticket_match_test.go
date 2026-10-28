package plane

import "testing"

func TestFindIssueByTicketIDNeverMatchesSubstrings(t *testing.T) {
	issues := []PlaneIssue{
		{ID: "u-1014", Name: "[DEN-1014] feat(agentjobs): worktrees"},
		{ID: "u-100", Name: "DEN-100 legacy name without brackets"},
		{ID: "u-10", Name: "[DEN-10] the real one"},
	}
	cases := map[string]string{
		"DEN-10":  "u-10",   // bracketed prefix wins even though DEN-1014 comes first
		"DEN-100": "u-100",  // whole-token fallback for unbracketed names
		"u-1014":  "u-1014", // UUID lookup
	}
	for id, want := range cases {
		got := findIssueByTicketID(issues, id)
		if got == nil || got.ID != want {
			t.Errorf("findIssueByTicketID(%q) = %+v, want %s", id, got, want)
		}
	}
	for _, id := range []string{"DEN-1", "DEN-101", "EN-10"} {
		if got := findIssueByTicketID(issues, id); got != nil {
			t.Errorf("findIssueByTicketID(%q) matched %s by substring", id, got.Name)
		}
	}
}

func TestFindIssueByTicketIDSequenceOnlyForUnprefixedTitles(t *testing.T) {
	issues := []PlaneIssue{
		{ID: "u-a", SequenceID: 324, Name: "Fresh ticket without id prefix"},
		{ID: "u-b", SequenceID: 10, Name: "[DEN-1014] legacy id differs from sequence"},
	}
	if got := findIssueByTicketID(issues, "DEN-324"); got == nil || got.ID != "u-a" {
		t.Errorf("DEN-324 = %+v, want u-a", got)
	}
	if got := findIssueByTicketID(issues, "DEN-10"); got != nil {
		t.Errorf("DEN-10 matched titled issue %s by sequence", got.Name)
	}
}

func TestResolveIssueNeverSubstring(t *testing.T) {
	issues := []PlaneIssue{
		{ID: "u-1014", SequenceID: 300, Name: "[DEN-1014] worktrees"},
		{ID: "u-new", SequenceID: 324, Name: "fresh"},
	}
	if got := ResolveIssue(issues, "DEN-1"); got != nil {
		t.Errorf("DEN-1 resolved to %s", got.Name)
	}
	if got := ResolveIssue(issues, "324"); got == nil || got.ID != "u-new" {
		t.Errorf("bare 324 = %+v", got)
	}
	if got := ResolveIssue(issues, "DEN-324"); got == nil || got.ID != "u-new" {
		t.Errorf("DEN-324 = %+v", got)
	}
}
