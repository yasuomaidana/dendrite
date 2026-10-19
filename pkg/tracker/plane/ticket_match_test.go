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
