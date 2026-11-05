package directive

import "strings"

// Diff shows what changed between two texts: removed lines with "-", added lines with "+", and one unchanged
// line of context around each change. It is empty when nothing changed.
func Diff(oldText, newText string) string {
	a, b := splitLines(oldText), splitLines(newText)
	// longest common subsequence table
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	type row struct {
		mark byte
		text string
	}
	var rows []row
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			rows, i, j = append(rows, row{' ', a[i]}), i+1, j+1
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			rows, i = append(rows, row{'-', a[i]}), i+1
		default:
			rows, j = append(rows, row{'+', b[j]}), j+1
		}
	}
	show := make([]bool, len(rows))
	changed := false
	for k, r := range rows {
		if r.mark != ' ' {
			changed = true
			for c := max(0, k-1); c <= min(len(rows)-1, k+1); c++ {
				show[c] = true
			}
		}
	}
	if !changed {
		return ""
	}
	var out strings.Builder
	gap := false
	for k, r := range rows {
		if !show[k] {
			gap = true
			continue
		}
		if gap && out.Len() > 0 {
			out.WriteString("  ...\n")
		}
		gap = false
		out.WriteString(string(r.mark) + " " + r.text + "\n")
	}
	return out.String()
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
