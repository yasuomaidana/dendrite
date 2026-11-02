package devtool

import "testing"

func TestExtractTicketIDAndTitleAnyProjectIdentifier(t *testing.T) {
	cases := []struct{ in, id, title string }{
		{"[DEN-324] Fix resolver", "DEN-324", "Fix resolver"},
		{"[KAKEBO-12] Import ledger", "KAKEBO-12", "Import ledger"},
		{"DRAG-7 index tickets", "DRAG-7", "index tickets"},
		{"Support UTF-8 input", "", "Support UTF-8 input"},
		{"legacy name DEN-100 mention", "DEN-100", "legacy name  mention"},
		{"[DEN-101] Fix bug in parser", "DEN-101", "Fix bug in parser"},
		{"[DEND-202] Add graph node", "DEND-202", "Add graph node"},
		{"DEN-303 Fix without brackets", "DEN-303", "Fix without brackets"},
	}
	for _, c := range cases {
		id, title := extractTicketIDAndTitle(c.in)
		if id != c.id || title != c.title {
			t.Errorf("%q = (%q, %q), want (%q, %q)", c.in, id, title, c.id, c.title)
		}
	}
}
