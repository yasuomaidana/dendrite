// Package textutil holds small string helpers shared by the commands and servers.
package textutil

import "strings"

// FirstLine returns the first line of s, trimmed, cut to max runes and ending in "…" when cut.
func FirstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}
