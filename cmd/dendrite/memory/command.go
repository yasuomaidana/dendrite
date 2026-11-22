// Package memory is `dendrite memory`: the agent-memory write path. Agents and hooks journal what is worth
// remembering; the journal is flushed into Engram when a session ends, and the memory mode says which
// stores are in use.
package memory

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/memory"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/statedir"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// NewMemoryCmd builds `dendrite memory`.
func NewMemoryCmd() *cobra.Command {
	cmd := cli.NewGroupCmd("memory", "Agent memory: journal what is worth remembering and flush it into Engram", "cortex")
	cmd.Example = "dendrite memory save --type decision \"Use X\" \"because Y\"\n" +
		"dendrite memory flush\n" +
		"dendrite memory mode\n" +
		"dendrite memory mode set engram\n" +
		"dendrite memory sync"
	cmd.AddCommand(newSaveCmd(), newFlushCmd(), newModeCmd(), newSyncCmd())
	return cmd
}

// claudeRepo is the repository whose Claude Code memory folder `sync` reads by default (a seam for tests).
var claudeRepo = func() string {
	out, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if top := strings.TrimSpace(string(out)); err == nil && top != "" {
		return strings.TrimSuffix(top, "/.git")
	}
	wd, _ := os.Getwd()
	return wd
}

func newSaveCmd() *cobra.Command {
	var typ, ticket string
	cmd := &cobra.Command{
		Use:   "save <title> <body>",
		Short: "Journal one memory now; it reaches Engram at the next flush (session end or `memory flush`)",
		Args:  cobra.ExactArgs(2),
		Example: "dendrite memory save --type decision \"Engram is the memory SoT\" \"measured weekly against Claude memory\"\n" +
			"types: decision, plan, note, outcome, or any label Engram accepts",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := projectid.HomeDir()
			if err != nil {
				return err
			}
			mode := memory.LoadState(home).Resolve()
			if !mode.Engram() {
				fmt.Fprintf(cmd.OutOrStdout(), "[i] memory mode is %q: Engram is not written to, nothing journaled\n", mode)
				return nil
			}
			e, err := memory.DefaultJournal().Append(memory.Entry{Type: typ, Ticket: ticket, Title: args[0], Body: args[1], Source: "memory save"})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[✓] Journaled %s (%s); `dendrite memory flush` or the session end sends it to Engram\n", e.ID, e.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "note", "Memory type (decision, plan, note, outcome, ...)")
	cmd.Flags().StringVar(&ticket, "ticket", "", "Ticket the memory belongs to")
	return cmd
}

// noArgsCmd is a command without arguments that runs run.
func noArgsCmd(use, short string, run func(cmd *cobra.Command) error) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cli.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return run(cmd) }}
}

func newFlushCmd() *cobra.Command {
	return noArgsCmd("flush", "Send the journaled memories to Engram now", func(cmd *cobra.Command) error {
		home, err := projectid.HomeDir()
		if err != nil {
			return err
		}
		n, err := memory.FlushPending(cmd.Context(), home, memory.DefaultJournal())
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[✓] %d memory item(s) flushed to Engram (journal %s)\n", n, statedir.Resolve(".dendrite/session/memory-journal.jsonl"))
		return nil
	})
}

func newModeCmd() *cobra.Command {
	cmd := noArgsCmd("mode", "Show which memory stores are in use, and why", func(cmd *cobra.Command) error {
		home, err := projectid.HomeDir()
		if err != nil {
			return err
		}
		return printMode(cmd.OutOrStdout(), memory.LoadState(home))
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set <both|engram|claude|off|auto>",
		Short: "Force a memory mode by hand (auto removes the override)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := memory.ParseMode(args[0])
			if err != nil {
				return cli.UsageErrorf("%s", err.Error())
			}
			home, err := projectid.HomeDir()
			if err != nil {
				return err
			}
			if err := memory.SetOverride(home, mode); err != nil {
				return err
			}
			return printMode(cmd.OutOrStdout(), memory.LoadState(home))
		},
	})
	return cmd
}

// printMode reports the mode in force and where it came from.
func printMode(out io.Writer, s memory.State) error {
	source := "default"
	switch {
	case s.Override != "":
		source = "override"
	case s.Computed != "":
		source = "computed: " + s.Reason
	}
	if cli.Globals.JSON {
		return cli.WriteJSON(out, map[string]any{"mode": s.Resolve(), "source": source, "override": s.Override, "computed": s.Computed})
	}
	fmt.Fprintf(out, "[✓] Memory mode: %s (%s)\n", s.Resolve(), source)
	return nil
}

func newSyncCmd() *cobra.Command {
	var dir string
	cmd := noArgsCmd("sync", "Copy Claude Code's file memories into Engram (one way, idempotent, never changes the files)", func(cmd *cobra.Command) error {
		return runSync(cmd.Context(), dir, cmd.OutOrStdout())
	})
	cmd.Flags().StringVar(&dir, "dir", "", "Claude memory folder (default ~/.claude/projects/<repo>/memory)")
	return cmd
}

func runSync(ctx context.Context, dir string, out io.Writer) error {
	home, err := projectid.HomeDir()
	if err != nil {
		return err
	}
	if dir == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = memory.ClaudeDir(userHome, claudeRepo())
	}
	n, err := memory.MirrorClaude(ctx, home, dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] %d Claude memory file(s) mirrored into Engram from %s\n", n, dir)
	return nil
}
