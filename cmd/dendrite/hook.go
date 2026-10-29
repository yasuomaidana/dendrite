package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/searchguard"
)

// searchGuardDir is where the guard keeps its state and the usage log: Dendrite's home, so one log
// covers every project an agent works in.
func searchGuardDir() string {
	home, err := projectid.HomeDir()
	if err != nil {
		return filepath.Join(".dendrite", "state")
	}
	return filepath.Join(home, "state")
}

// newHookCmd builds `dendrite hook`: entry points that agent runtimes call around their tools.
func newHookCmd() *cobra.Command {
	cmd := newGroupCmd("hook", "Entry points agent runtimes call around their tools (not for typing by hand)", "boutons")
	cmd.Example = "# .claude/settings.json (written by `dendrite soma prepare`)\n" +
		"#   \"PreToolUse\": [{\"matcher\": \"Bash|Grep|mcp__dendrite.*\", \"hooks\": [{\"type\": \"command\", \"command\": \"dendrite hook pre-tool\"}]}]"
	cmd.AddCommand(&cobra.Command{
		Use:   "pre-tool",
		Short: "Pre-tool hook: stop a raw recursive search once, point at the Dendrite equivalent, and log tool use",
		Long: "Reads a Claude Code PreToolUse event on stdin. The first time a session runs a raw recursive search\n" +
			"(rg, grep -r, the Grep tool) it exits 2 with the Dendrite equivalents on stderr; repeating the same\n" +
			"command goes through. " + searchguard.AllowEnv + "=1 bypasses. Dendrite calls are only counted\n" +
			"(see `dendrite sensory usage`). Any malformed input is allowed: a hook must never break the agent.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreTool(cmd.InOrStdin(), searchguard.New(searchGuardDir()))
		},
	})
	cmd.AddCommand(ticketHookCmds()...)
	return cmd
}

func runPreTool(stdin io.Reader, g *searchguard.Guard) error {
	data, err := io.ReadAll(io.LimitReader(stdin, 4<<20))
	if err != nil {
		return nil
	}
	var in searchguard.HookInput
	if json.Unmarshal(data, &in) != nil || in.ToolName == "" {
		return nil
	}
	if d := g.Check(in); d.Block {
		return usageErrorf("%s", d.Message)
	}
	return nil
}

// runSensoryUsage prints how an agent's tool use splits between Dendrite and raw search.
func runSensoryUsage(out io.Writer, since time.Duration) error {
	g := searchguard.New(searchGuardDir())
	f, err := openIfExists(g.LogPath())
	if err != nil {
		return err
	}
	if f == nil {
		fmt.Fprintln(out, "[i] No tool use recorded yet: run `dendrite soma prepare` in a project to install the agent hook.")
		return nil
	}
	defer f.Close()
	s := searchguard.Summarize(f, time.Now().Add(-since))
	if globals.JSON {
		return writeJSON(out, s)
	}
	raw := 0
	for _, n := range s.RawSearches {
		raw += n
	}
	window := since.Round(time.Hour).String()
	if since >= 24*time.Hour {
		window = fmt.Sprintf("%.0f days", since.Hours()/24)
	}
	fmt.Fprintf(out, "[+] Agent tool use over the last %s:\n", window)
	fmt.Fprintf(out, "    - Dendrite calls:        %d\n", s.Dendrite)
	fmt.Fprintf(out, "    - Raw searches that ran: %d", raw)
	for _, k := range searchguard.Kinds(s.RawSearches) {
		fmt.Fprintf(out, "  %s=%d", k, s.RawSearches[k])
	}
	fmt.Fprintln(out)
	blocked := 0
	for _, n := range s.Blocked {
		blocked += n
	}
	fmt.Fprintf(out, "    - First attempts stopped: %d", blocked)
	for _, k := range searchguard.Kinds(s.Blocked) {
		fmt.Fprintf(out, "  %s=%d", k, s.Blocked[k])
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "    - Dendrite share:        %.0f%% of retrieval calls\n", s.Share*100)
	return nil
}

// openIfExists opens path, returning nil (and no error) when it does not exist.
func openIfExists(path string) (*os.File, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return f, err
}
