package bootstrap

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/yasuomaidana/dendrite-mcp-hub/pkg/jetbrains"
)

// fakeAGY replaces the agy command for one test: installed says whether it is on PATH, answer
// gives each call's output and error, and the calls it received are returned.
func fakeAGY(t *testing.T, installed bool, answer func(args []string) ([]byte, error)) *[][]string {
	t.Helper()
	oldLook, oldRun := agyLookPath, agyRun
	t.Cleanup(func() { agyLookPath, agyRun = oldLook, oldRun })
	agyLookPath = func(string) (string, error) {
		if !installed {
			return "", exec.ErrNotFound
		}
		return "/fake/agy", nil
	}
	var calls [][]string
	agyRun = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		return answer(args)
	}
	return &calls
}

func twoServers() []mcpServer {
	return []mcpServer{
		{name: "dendrite", entry: jetbrains.MCPServerEntry{Command: "dendrite", Args: []string{"inter", "serve", "--profile", "core"}}},
		{name: "dendrite-synapse-x", entry: jetbrains.MCPServerEntry{Command: "dendrite", Args: []string{"inter", "serve", "--synapse", "x"}}},
	}
}

func TestRegisterAGYServersIsSilentWithoutAGY(t *testing.T) {
	calls := fakeAGY(t, false, nil)
	if got, note := registerAGYServers(twoServers()); got != nil || note != "" || len(*calls) != 0 {
		t.Errorf("got %v %q with %d calls, want silence", got, note, len(*calls))
	}
}

func TestRegisterAGYServersAddsEveryServerWithItsCommand(t *testing.T) {
	calls := fakeAGY(t, true, func([]string) ([]byte, error) { return nil, nil })
	got, note := registerAGYServers(twoServers())
	if note != "" || !reflect.DeepEqual(got, []string{"dendrite", "dendrite-synapse-x"}) {
		t.Errorf("registered %v, note %q", got, note)
	}
	want := []string{"mcp", "add", "dendrite", "--", "dendrite", "inter", "serve", "--profile", "core"}
	if !reflect.DeepEqual((*calls)[0], want) {
		t.Errorf("first call = %v, want %v", (*calls)[0], want)
	}
}

func TestRegisterAGYServersStopsAtARefusalAndNamesTheServer(t *testing.T) {
	fakeAGY(t, true, func(args []string) ([]byte, error) {
		if args[2] == "dendrite-synapse-x" {
			return []byte("not logged in\n"), errors.New("exit 1")
		}
		return nil, nil
	})
	got, note := registerAGYServers(twoServers())
	if !reflect.DeepEqual(got, []string{"dendrite"}) {
		t.Errorf("registered %v", got)
	}
	expectContains(t, note, "dendrite-synapse-x", "not logged in")
}

func TestRegisterAGYRegistersTheRepositorysOwnServers(t *testing.T) {
	calls := fakeAGY(t, true, func([]string) ([]byte, error) { return nil, nil })
	got, _ := RegisterAGY(t.TempDir())
	if len(got) == 0 || got[0] != "dendrite" || len(*calls) != len(got) {
		t.Errorf("registered %v with %d calls", got, len(*calls))
	}
}

// expectContains fails for every text that s does not contain.
func expectContains(t *testing.T, s string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("missing %q in %q", w, s)
		}
	}
}

func TestAGYMCPStatus(t *testing.T) {
	cases := []struct {
		name      string
		installed bool
		out       string
		runErr    error
		want      AGYMCPState
	}{
		{"agy is not installed", false, "", nil, AGYAbsent},
		{"the server is listed", true, "Configured MCP servers:\n  ✓ dendrite (stdio)\n", nil, AGYRegistered},
		{"only other servers are listed", true, "  fs (stdio)\n  dendrite-synapse-x (stdio)\n", nil, AGYNotRegistered},
		{"nothing is configured", true, "No MCP servers configured.\n", nil, AGYNotRegistered},
		{"the list cannot be read", true, "boom", errors.New("exit 2"), AGYUnreadable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeAGY(t, c.installed, func([]string) ([]byte, error) { return []byte(c.out), c.runErr })
			got, err := AGYMCPStatus("dendrite")
			if got != c.want || (err != nil) != (c.runErr != nil) {
				t.Errorf("state %v err %v, want %v", got, err, c.want)
			}
		})
	}
}
