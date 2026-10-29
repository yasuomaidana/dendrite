package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
)

// hookDeps builds the dependencies of the ticket hooks (a seam for tests).
var hookDeps = ticketlock.HookDeps

// ticketHookCmds are the hooks of the ticket lock: the write guard, the plan saver, the session-end handoff
// and the git pre-commit guard. They are installed by `dendrite soma prepare`.
func ticketHookCmds() []*cobra.Command {
	preEdit := &cobra.Command{
		Use:   "pre-edit",
		Short: "Pre-edit hook: refuse a file change unless this working copy has a claimed, In Progress ticket",
		Long: "Reads a Claude Code PreToolUse event for Edit, Write, MultiEdit or NotebookEdit on stdin. A write inside a\n" +
			"repository that is a Plane project needs a session token and a claim for the branch checked out there, and\n" +
			"the claimed ticket must be In Progress (Plane is asked at most every 10 minutes; an unreachable Plane is\n" +
			"covered by a cached claim under 24 hours old). Without them it exits 2 and says how to claim. Writes outside\n" +
			"any repository, in repositories that are not Plane projects, under .dendrite/ and in ~/.claude/plans/ are\n" +
			"allowed. Malformed input is allowed: only a judged write is ever refused.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreEdit(cmd.Context(), cmd.InOrStdin(), hookDeps())
		},
	}
	preCommit := &cobra.Command{
		Use:   "pre-commit",
		Short: "Git pre-commit hook: the same guard for every staged path, so agents cannot bypass it through git",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreCommit(cmd.Context(), hookDeps(), ".")
		},
	}
	planSaved := &cobra.Command{
		Use:   "plan-saved",
		Short: "Post-ExitPlanMode hook: store the plan on the claimed ticket (kept locally when there is none)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlanSaved(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	sessionEnd := &cobra.Command{
		Use:   "session-end",
		Short: "Session-end hook: a ticket still In Progress gets an automatic handoff and is blocked (needs-input)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionEnd(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	return []*cobra.Command{preEdit, preCommit, planSaved, sessionEnd}
}

// runPreEdit refuses a write the lock does not allow. Anything it cannot judge is allowed.
func runPreEdit(ctx context.Context, stdin io.Reader, d ticketlock.Deps) error {
	in, ok := ticketlock.ParseHookInput(stdin)
	if !ok {
		return nil
	}
	if err := ticketlock.CheckEdit(ctx, d, in); err != nil {
		return usageErrorf("%s", err.Error())
	}
	return nil
}

// runPreCommit refuses a commit whose staged paths the lock does not allow.
func runPreCommit(ctx context.Context, d ticketlock.Deps, dir string) error {
	if err := ticketlock.CheckStaged(ctx, d, dir); err != nil {
		return usageErrorf("%s", err.Error())
	}
	return nil
}

// boardOfDir opens the Plane board of the repository containing dir, when a hook needs it.
func boardOfDir(dir string) func() (ticketlock.Board, error) {
	return func() (ticketlock.Board, error) {
		top, err := plane.FindRepoRoot(dir)
		if err != nil {
			return ticketlock.Board{}, err
		}
		return ticketlock.BoardFor(top)
	}
}

// reportHook prints what a hook did. Hooks after the fact never fail the agent, so a problem is only reported.
func reportHook(out io.Writer, msg string, err error) {
	switch {
	case err != nil:
		fmt.Fprintf(out, "[!] %v\n", err)
	case msg != "":
		fmt.Fprintf(out, "[✓] %s\n", msg)
	}
}

func runPlanSaved(ctx context.Context, stdin io.Reader, out io.Writer) error {
	in, ok := ticketlock.ParseHookInput(stdin)
	if !ok {
		return nil
	}
	l := ticketlock.NewLifecycle(ticketlock.Board{}, hookDir(in))
	msg, err := ticketlock.SavePlan(ctx, l, boardOfDir(l.Dir), in)
	reportHook(out, msg, err)
	return nil
}

func runSessionEnd(ctx context.Context, stdin io.Reader, out io.Writer) error {
	in, ok := ticketlock.ParseHookInput(stdin)
	if !ok {
		return nil
	}
	l := ticketlock.NewLifecycle(ticketlock.Board{}, hookDir(in))
	msg, err := ticketlock.AutoHandoff(ctx, l, boardOfDir(l.Dir), in)
	reportHook(out, msg, err)
	return nil
}

func hookDir(in ticketlock.HookInput) string {
	if in.Cwd == "" {
		return "."
	}
	return in.Cwd
}
