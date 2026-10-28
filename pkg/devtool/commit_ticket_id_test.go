package devtool

import "testing"

func TestExtractTicketIDAndTitleAnyProjectIdentifier(t *testing.T) {
	cases := []struct{ in, id, title string }{
		{"[DEN-324] Fix resolver", "DEN-324", "Fix resolver"},
		{"[KAKEBO-12] Import ledger", "KAKEBO-12", "Import ledger"},
		{"DRAG-7 index tickets", "DRAG-7", "index tickets"},
		{"Support UTF-8 input", "", "Support UTF-8 input"},
		{"legacy name DEN-100 mention", "DEN-100", "legacy name  mention"},
	}
	for _, c := range cases {
		id, title := extractTicketIDAndTitle(c.in)
		if id != c.id || title != c.title {
			t.Errorf("%q = (%q, %q), want (%q, %q)", c.in, id, title, c.id, c.title)
		}
	}
}
