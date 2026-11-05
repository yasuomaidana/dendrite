package bootstrap

import (
	"fmt"
	"os/exec"
	"strings"
)

// agyLookPath and agyRun are the seams to the Antigravity CLI: tests replace them, so no test touches
// the real agy registry.
var (
	agyLookPath = exec.LookPath
	agyRun      = func(args ...string) ([]byte, error) { return exec.Command("agy", args...).CombinedOutput() }
)

// RegisterAGY registers the servers the repository at root should have (see mcpServersFor) with AGY.
func RegisterAGY(root string) (registered []string, note string) {
	return registerAGYServers(mcpServersFor(root))
}

// registerAGYServers adds the servers to AGY's own registry. AGY does not read .agents/mcp.json: it keeps
// its servers in a registry managed by `agy mcp add`, so without this an AGY session has no dendrite tools
// and falls back to editing files by script. `agy mcp add` adds or updates, so running it twice is safe.
// It returns the names registered and a note when AGY is missing or refused a server.
func registerAGYServers(servers []mcpServer) (registered []string, note string) {
	if _, err := agyLookPath("agy"); err != nil {
		return nil, ""
	}
	for _, s := range servers {
		args := append([]string{"mcp", "add", s.name, "--", s.entry.Command}, s.entry.Args...)
		if out, err := agyRun(args...); err != nil {
			return registered, fmt.Sprintf("agy mcp add %s: %v: %s", s.name, err, strings.TrimSpace(string(out)))
		}
		registered = append(registered, s.name)
	}
	return registered, ""
}

// AGYMCPState is whether AGY can reach the Dendrite MCP server.
type AGYMCPState int

const (
	// AGYAbsent: the agy command is not installed.
	AGYAbsent AGYMCPState = iota
	// AGYRegistered: `agy mcp list` names the server.
	AGYRegistered
	// AGYNotRegistered: agy answers but does not list the server.
	AGYNotRegistered
	// AGYUnreadable: agy is installed but `agy mcp list` failed.
	AGYUnreadable
)

// AGYMCPStatus reports whether the named server is in AGY's registry.
func AGYMCPStatus(name string) (AGYMCPState, error) {
	if _, err := agyLookPath("agy"); err != nil {
		return AGYAbsent, nil
	}
	out, err := agyRun("mcp", "list")
	if err != nil {
		return AGYUnreadable, fmt.Errorf("agy mcp list: %w: %s", err, strings.TrimSpace(string(out)))
	}
	for _, line := range strings.Split(string(out), "\n") {
		for _, field := range strings.Fields(line) {
			if strings.Trim(field, ":*-•✓[]()") == name {
				return AGYRegistered, nil
			}
		}
	}
	return AGYNotRegistered, nil
}
