package textutil

import (
	"strings"
	"testing"
)

func TestFirstLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"trims and keeps one line", "  one\ntwo", 100, "one"},
		{"short text is unchanged", "hello", 10, "hello"},
		{"exactly max is unchanged", "abcde", 5, "abcde"},
		{"longer text is cut with an ellipsis", "abcdef", 5, "abcde…"},
		{"cuts on runes, never inside one", "ééééé", 2, "éé…"},
		{"empty stays empty", " \n ", 5, ""},
		{"a long first line only", strings.Repeat("x", 12) + "\nrest", 10, strings.Repeat("x", 10) + "…"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FirstLine(c.in, c.max); got != c.want {
				t.Fatalf("FirstLine(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
			}
		})
	}
}
