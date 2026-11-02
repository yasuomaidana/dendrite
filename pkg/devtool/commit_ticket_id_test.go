package devtool

import (
	"testing"

	"Dendrite/pkg/testkit"
)

func TestExtractTicketIDAndTitleAnyProjectIdentifier(t *testing.T) {
	testkit.Table(t, map[string][2]string{
		"[DEN-324] Fix resolver":       {"DEN-324", "Fix resolver"},
		"[KAKEBO-12] Import ledger":    {"KAKEBO-12", "Import ledger"},
		"DRAG-7 index tickets":         {"DRAG-7", "index tickets"},
		"Support UTF-8 input":          {"", "Support UTF-8 input"},
		"legacy name DEN-100 mention":  {"DEN-100", "legacy name  mention"},
		"[DEN-101] Fix bug in parser":  {"DEN-101", "Fix bug in parser"},
		"[DEND-202] Add graph node":    {"DEND-202", "Add graph node"},
		"DEN-303 Fix without brackets": {"DEN-303", "Fix without brackets"},
		"No ticket in this title":      {"", "No ticket in this title"},
	}, func(in string) [2]string { id, title := extractTicketIDAndTitle(in); return [2]string{id, title} })
}
