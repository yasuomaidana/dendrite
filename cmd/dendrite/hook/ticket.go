package hook

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/devtool"
	"Dendrite/pkg/directive"
	"Dendrite/pkg/immune"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
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
			"allowed. Malformed input is allowed: only a judged write is ever refused.\n" +
			"A write into a skill under .claude/skills/ or .agents/skills/ that is committed at HEAD is also refused\n" +
			"while the skill's current content has no cleared scan in the ledger (`dendrite immune scan <skill>`); a new\n" +
			"skill may be written and is judged when it is committed.\n" +
			"Writes made through the shell (sed -i, redirects) are not seen here; the git pre-commit hook is the backstop.\n" +
			"The bypass for both hooks is " + devtool.TicketGuardEnv + "=off (quantal --no-ticket-guard sets it for its commit).",
		Args: cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreEdit(cmd.Context(), cmd.InOrStdin(), hookDeps())
		},
	}
	preCommit := &cobra.Command{
		Use:   "pre-commit",
		Short: "Git pre-commit hook: the same guard for every staged path, so agents cannot bypass it through git",
		Long: "Applies the ticket lock to the staged paths, and refuses a commit that stages files of a skill under\n" +
			".claude/skills/ or .agents/skills/ whose current content has no cleared scan in the ledger: approved by\n" +
			"`dendrite immune scan`, or held and accepted with `dendrite immune allow`. " + devtool.TicketGuardEnv + "=off bypasses\n" +
			"the ticket lock only, never the skill gate.",
		Args: cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreCommit(cmd.Context(), hookDeps(), ".")
		},
	}
	planSaved := &cobra.Command{
		Use:   "plan-saved",
		Short: "Post-ExitPlanMode hook: store the plan on the claimed ticket (kept locally when there is none)",
		Args:  cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlanSaved(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	sessionEnd := &cobra.Command{
		Use:   "session-end",
		Short: "Session-end hook: a ticket still In Progress gets an automatic handoff and is blocked (needs-input)",
		Args:  cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionEnd(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	decision := &cobra.Command{
		Use:   "decision",
		Short: "Post-AskUserQuestion hook: record the questions and the user's answers on the claimed ticket (kept locally until a claim)",
		Args:  cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDecision(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	return []*cobra.Command{preEdit, preCommit, planSaved, sessionEnd, decision}
}

// runPreEdit refuses a write the ticket lock or the immune gate does not allow. Anything it cannot
// judge is allowed.
func runPreEdit(ctx context.Context, stdin io.Reader, d ticketlock.Deps) error {
	in, ok := ticketlock.ParseHookInput(stdin)
	if !ok {
		return nil
	}
	if err := ticketlock.CheckEdit(ctx, d, in); err != nil {
		return cli.UsageErrorf("%s", err.Error())
	}
	if target := in.Target(); target != "" {
		return refusal(immuneGate(target).CheckEdit(target))
	}
	return nil
}

// runPreCommit refuses a commit whose staged paths the ticket lock does not allow, or that stages
// a skill the immune gate has not cleared. The ticket guard's bypass does not switch the immune
// gate off.
func runPreCommit(ctx context.Context, d ticketlock.Deps, dir string) error {
	if devtool.GuardEnabled() { // off: the deliberate bypass, --no-ticket-guard or DENDRITE_TICKET_GUARD=off
		if err := ticketlock.CheckStaged(ctx, d, dir); err != nil {
			return cli.UsageErrorf("%s", err.Error())
		}
	}
	return refusal(immuneGate(dir).CheckStaged(dir))
}

// immuneGate is the skill gate of the repository holding path, with the scanner version its
// directives pin. Directives that cannot be read pin nothing.
func immuneGate(path string) immune.Gate {
	set, err := directive.Load(immune.RepoTop(path))
	if err != nil {
		return immune.Gate{}
	}
	return immune.Gate{Pin: set.Immune.ScannerVersion}
}

// refusal turns a gate's error into the exit code 2 an agent runtime treats as "stop".
func refusal(err error) error {
	if err == nil {
		return nil
	}
	return cli.UsageErrorf("blocked: %s", err.Error())
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

// runAfterHook parses the hook input from stdin, runs act on its working copy's board and reports
// the outcome. An unreadable input is ignored: these hooks never fail the agent.
func runAfterHook(ctx context.Context, stdin io.Reader, out io.Writer, act hookAct) error {
	in, ok := ticketlock.ParseHookInput(stdin)
	if !ok {
		return nil
	}
	l := ticketlock.NewLifecycle(ticketlock.Board{}, hookDir(in))
	msg, err := act(ctx, l, boardOfDir(l.Dir), in)
	reportHook(out, msg, err)
	return nil
}

func runPlanSaved(ctx context.Context, stdin io.Reader, out io.Writer) error {
	return runAfterHook(ctx, stdin, out, journaling(ticketlock.SavePlan, planEntries))
}

func runSessionEnd(ctx context.Context, stdin io.Reader, out io.Writer) error {
	return runAfterHook(ctx, stdin, out, flushing(ticketlock.AutoHandoff))
}

func runDecision(ctx context.Context, stdin io.Reader, out io.Writer) error {
	return runAfterHook(ctx, stdin, out, journaling(ticketlock.SaveDecisions, decisionEntries))
}

func hookDir(in ticketlock.HookInput) string {
	if in.Cwd == "" {
		return "."
	}
	return in.Cwd
}
