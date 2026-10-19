package main

import (
	"strings"
	"testing"
)

func TestGreeting(t *testing.T) {
	steps := []struct {
		args []string
		want string
	}{
		{[]string{"greeting", "--topic", "DEN-985"}, "Dynamic 10-Style Greeting Matrix Resolution"},
		{[]string{"greet", "--event", "launch", "--days", "3", "--stress", "--options", "a,b", "--suppress", "G-01, G-02", "--json"}, `"headline"`},
		{[]string{"greeting", "--milestone", "M3", "--hobby", "--days-inactive", "9", "--turns", "4", "--unfinished", "x", "--challenge", "y", "--curiosity", "z", "--decision", "d"}, "Headline:"},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != ExitOK || !strings.Contains(strings.ToLower(out), strings.ToLower(s.want)) {
			t.Errorf("%v: exit %d\n%s", s.args, code, out)
		}
	}
	if _, code := cliOutput(t, "greeting", "extra"); code != ExitUsage {
		t.Errorf("positional argument: exit %d", code)
	}
}
