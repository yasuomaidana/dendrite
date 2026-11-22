package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"Dendrite/pkg/mcpserver"
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

// reaToolsTimeout bounds the start of the server and its answer to tools/list.
const reaToolsTimeout = 30 * time.Second

// mcpExchange is the seam that runs an MCP server over stdio: it writes input, closes the server's stdin and
// returns what the server printed. Tests replace it, so no test starts the real server.
var mcpExchange = func(ctx context.Context, command string, args []string, input string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.Bytes(), err
}

// mcpListRequests initialize the session and ask for the tool list (answer id 2).
const mcpListRequests = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"dendrite-doctor","version":"0"}}}` + "\n" +
	`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
	`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"

// AGYReaTools starts the dendrite MCP server of the repository at root exactly as AGY is told to start it
// (see mcpServersFor) and returns the names of the dendrite_rea_ tools it serves. An empty list means AGY
// is registered but would not see the REA tools.
func AGYReaTools(root string) ([]string, error) {
	entry := mcpServersFor(root)[0].entry
	ctx, cancel := context.WithTimeout(context.Background(), reaToolsTimeout)
	defer cancel()
	out, err := mcpExchange(ctx, entry.Command, entry.Args, mcpListRequests)
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("start %s: %w", entry.Command, err)
	}
	return reaToolNames(out)
}

// reaToolNames reads the tools/list answer (id 2) from the server's output and keeps the REA tools.
func reaToolNames(out []byte) ([]string, error) {
	for _, line := range strings.Split(string(out), "\n") {
		var resp struct {
			ID     int `json:"id"`
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &resp) != nil || resp.ID != 2 {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("tools/list: %s", resp.Error.Message)
		}
		var names []string
		for _, t := range resp.Result.Tools {
			if strings.HasPrefix(t.Name, mcpserver.ReaToolPrefix) {
				names = append(names, t.Name)
			}
		}
		return names, nil
	}
	return nil, fmt.Errorf("the server did not answer tools/list")
}
