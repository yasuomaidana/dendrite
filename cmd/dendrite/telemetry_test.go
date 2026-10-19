package main

import (
	"strings"
	"testing"
)

func TestSensory(t *testing.T) {
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"sensory"}, ExitOK, "Instantaneous Telemetry Snapshot"},
		{[]string{"telemetry", "snapshot", "--json"}, ExitOK, `"node_status"`},
		{[]string{"sensory", "metrics"}, ExitOK, "# "},
		{[]string{"sensory", "prometheus"}, ExitOK, "# "},
		{[]string{"sensory", "bogus"}, ExitUsage, ""},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
}
