package hook

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"strings"
	"testing"
)

func TestSensory(t *testing.T) {
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"sensory"}, cli.ExitOK, "Instantaneous Telemetry Snapshot"},
		{[]string{"telemetry", "snapshot", "--json"}, cli.ExitOK, `"node_status"`},
		{[]string{"sensory", "metrics"}, cli.ExitOK, "# "},
		{[]string{"sensory", "prometheus"}, cli.ExitOK, "# "},
		{[]string{"sensory", "bogus"}, cli.ExitUsage, ""},
	}
	for _, s := range steps {
		out, code := cliOutput(t, s.args...)
		if code != s.code || !strings.Contains(out, s.want) {
			t.Errorf("%v: exit %d (want %d)\n%s", s.args, code, s.code, out)
		}
	}
}
