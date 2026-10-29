package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/ticketlock"

	"github.com/spf13/cobra"
)

// newQuantalCmd builds `dendrite quantal` (alias commit): ticket-aware commits with generated messages, then push.
func newQuantalCmd() *cobra.Command {
	opts := devtool.CommitOptions{UseLocalModel: true, Model: "qwen3.8:27b", Smart: true}
	var noSmart, noLocal bool
	var synapseID string
	cmd := &cobra.Command{
		Use:     "quantal",
		Aliases: []string{"commit", "quantal-commit", "cmt"},
		Short:   "Autonomous Git commit with local model message generation",
		Long: "Commits the staged changes (or --files / --all), references the ticket, and pushes to the remote.\n" +
			"It never pushes main/master; --no-push or DENDRITE_COMMIT_PUSH=false skips the push.",
		GroupID: "boutons",
		Args:    noArgs,
		Example: "dendrite quantal --ticket DEN-123\n" +
			"dendrite quantal -t DEN-123 --no-smart -m \"fix(cli): handle empty args\"\n" +
			"dendrite quantal --files cmd/a.go,cmd/b.go --dry-run\n" +
			"dendrite quantal --synapse -y",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Smart, opts.NoSmart = !noSmart, noSmart
			opts.UseLocalModel = !noLocal
			if strings.EqualFold(os.Getenv("DENDRITE_COMMIT_PUSH"), "false") {
				opts.NoPush = true
			}
			if !cmd.Flags().Changed("synapse") {
				return runQuantal(cmd.Context(), opts, cmd.OutOrStdout())
			}
			if synapseID == synapseAuto {
				synapseID = ""
			}
			return runQuantalSynapse(cmd.Context(), opts, synapseID, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	addTicketFlag(cmd, &opts.TicketID, "")
	f.StringVarP(&opts.Message, "message", "m", "", "Commit message (skips message generation)")
	f.StringSliceVarP(&opts.Files, "files", "f", nil, "Files to stage (comma-separated)")
	f.BoolVarP(&opts.StageAll, "all", "a", false, "Stage all modified and untracked files")
	f.BoolVar(&opts.Amend, "amend", false, "Amend the previous commit")
	f.BoolVar(&opts.NoPush, "no-push", false, "Do not push after committing")
	f.Bool("smart", true, "Resolve the active ticket from Plane when --ticket is not given (default)")
	f.BoolVar(&noSmart, "no-smart", false, "Do not resolve the ticket automatically")
	addModelFlag(cmd, &opts.Model, opts.Model)
	f.Bool("local", true, "Generate the message with the local model (default)")
	f.BoolVar(&noLocal, "no-local", false, "Do not use the local model for the message")
	addDryRunFlag(cmd, &opts.DryRun)
	f.BoolVar(&opts.AllowMocks, "allow-mocks", false, "Bypass the zero-mock gate (emergencies only)")
	f.BoolVar(&opts.NoTicketGuard, "no-ticket-guard", false, "Skip the ticket lifecycle check (the ticket must exist and be In Progress; env "+devtool.TicketGuardEnv+"=off)")
	f.StringVar(&synapseID, "synapse", "", "Then check every synapse peer (of this synapse, or every one holding the project): offer to commit dirty peers (-y: without asking), push peers ahead")
	f.Lookup("synapse").NoOptDefVal = synapseAuto
	cmd.MarkFlagsMutuallyExclusive("smart", "no-smart")
	cmd.MarkFlagsMutuallyExclusive("local", "no-local")
	cmd.MarkFlagsMutuallyExclusive("files", "all")
	return cmd
}

// synapseAuto is the --synapse value given without "=<id>": every synapse holding the project.
const synapseAuto = "auto"

// runQuantalSynapse commits the current project (nothing to commit is fine), then walks its
// synapse peers: dirty peers are committed after confirmation, peers ahead are pushed.
func runQuantalSynapse(ctx context.Context, opts devtool.CommitOptions, synapseID string, in io.Reader, out io.Writer) error {
	err := runQuantal(ctx, opts, out)
	if errors.Is(err, devtool.ErrNothingToCommit) {
		fmt.Fprintln(out, "[i] Nothing to commit in the current project; checking synapse peers...")
		err = nil
	}
	cwd, wdErr := os.Getwd()
	if wdErr != nil {
		return errors.Join(err, wdErr)
	}
	var confirm peerConfirm // nil: non-interactive without --yes, report only
	switch {
	case globals.Yes:
		confirm = func(string, int) bool { return true }
	case stdinIsTerminal():
		confirm = linePrompter(in, out)
	}
	return errors.Join(err, commitSynapsePeers(ctx, out, cwd, synapseID, opts, confirm))
}

func runQuantal(ctx context.Context, opts devtool.CommitOptions, out io.Writer) error {
	fmt.Fprintln(out, "[+] Initializing Autonomous Git Commit Engine...")
	if opts.TicketID != "" {
		fmt.Fprintf(out, "    - Active Ticket:    %s\n", opts.TicketID)
	} else if !opts.NoSmart {
		fmt.Fprintln(out, "    - Smart Strategy:   Auto-resolving active ticket and context from Plane / DAG...")
	}
	if opts.Amend {
		fmt.Fprintln(out, "    - Commit Mode:      AMEND (--amend)")
	}
	if opts.UseLocalModel && opts.Message == "" {
		fmt.Fprintf(out, "    - Message Model:    %s (Delegated Local Engine)\n", opts.Model)
	} else if opts.Message != "" {
		fmt.Fprintf(out, "    - Message Source:   User Specified\n")
	}
	if opts.DryRun {
		fmt.Fprintln(out, "    - Execution Mode:   DRY-RUN (Preview Only)")
	}

	res, err := devtool.ExecuteCommit(ctx, opts)
	if err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}

	pushFailed := printCommitResult(out, res)
	if !res.DryRun {
		postTicketComment(ctx, out, opts.RepoDir, "progress", commitProgress(res))
	}
	if pushFailed {
		return fmt.Errorf("git push failed: %w", res.PushError)
	}
	return nil
}

// commitProgress is the progress comment of a commit: its subject and short SHA.
func commitProgress(res *devtool.CommitResult) string {
	subject, _, _ := strings.Cut(strings.TrimSpace(res.Message), "\n")
	sha := res.CommitSHA
	if len(sha) > 7 {
		sha = sha[:7]
	}
	return fmt.Sprintf("%s (%s)", subject, sha)
}

// postTicketComment tells the ticket the working copy in dir has claimed what just happened. It is best
// effort: a copy with no claim posts nothing, and a failure is a warning that never undoes the work.
var postTicketComment = func(ctx context.Context, out io.Writer, dir, kind, body string) {
	if dir == "" {
		dir = "."
	}
	if _, claimed := ticketlock.NewStore().ClaimOfDir(dir); !claimed {
		return
	}
	client, err := newPlaneClient()
	if err == nil {
		_, err = lifecycleIn(client, dir).CommentClaimed(ctx, kind, body)
	}
	if err != nil {
		fmt.Fprintf(out, "[!] ticket comment not posted: %v\n", err)
	}
}

// printCommitResult renders a commit outcome and reports whether the push failed.
func printCommitResult(out io.Writer, res *devtool.CommitResult) bool {
	if res.DryRun {
		fmt.Fprintln(out, "[✓] Dry run commit preview generated:")
		if res.SmartTicket != nil && res.SmartTicket.ID != "" {
			fmt.Fprintf(out, "    - Smart Strategy:   Resolved [%s] via %s\n", res.SmartTicket.ID, res.SmartSource)
			if res.SmartTicket.Title != "" {
				fmt.Fprintf(out, "      * Title:          %s\n", res.SmartTicket.Title)
			}
		}
		printGuard(out, res.Guard)
		fmt.Fprintf(out, "    - Proposed Message:\n%s\n", indentText(res.Message, "      "))
		fmt.Fprintf(out, "    - Staged Files (%d):\n", len(res.Files))
		for _, f := range res.Files {
			fmt.Fprintf(out, "      * %s\n", f)
		}
		fmt.Fprintf(out, "    - Model Engine:     %s (Cost: $%.2f)\n", res.ModelUsed, res.CostUSD)
		if res.PushSkipped {
			fmt.Fprintf(out, "    - Remote Push:      Skipped (%s)\n", res.PushReason)
		}
		return false
	}

	fmt.Fprintf(out, "[✓] Commit applied successfully (%s)\n", res.CommitSHA[:min(8, len(res.CommitSHA))])
	if res.SmartTicket != nil && res.SmartTicket.ID != "" {
		fmt.Fprintf(out, "    - Smart Strategy:   Resolved [%s] via %s\n", res.SmartTicket.ID, res.SmartSource)
		if res.SmartTicket.Title != "" {
			fmt.Fprintf(out, "      * Title:          %s\n", res.SmartTicket.Title)
		}
	}
	printGuard(out, res.Guard)
	fmt.Fprintf(out, "    - Message:\n%s\n", indentText(res.Message, "      "))
	fmt.Fprintf(out, "    - Files Committed (%d):\n", len(res.Files))
	for _, f := range res.Files {
		fmt.Fprintf(out, "      * %s\n", f)
	}
	fmt.Fprintf(out, "    - Model Provenance: %s (Cost: $%.2f)\n", res.ModelUsed, res.CostUSD)
	if res.Pushed {
		fmt.Fprintln(out, "    - Remote Push:      Pushed to remote")
	} else if res.PushSkipped {
		fmt.Fprintf(out, "    - Remote Push:      Skipped (%s)\n", res.PushReason)
	}

	if res.PushError != nil {
		fmt.Fprintf(out, "[-] Git push failed: %v\n", res.PushError)
		return true
	}
	return false
}

func indentText(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = indent + l
	}
	return strings.Join(lines, "\n")
}

// printGuard reports what the ticket lifecycle guard did or why it could not check.
func printGuard(out io.Writer, g *devtool.GuardResult) {
	switch {
	case g == nil:
	case g.Note != "":
		fmt.Fprintf(out, "    - Ticket Guard:     %s\n", g.Note)
	case g.Skipped != "":
		fmt.Fprintf(out, "    - Ticket Guard:     not checked (%s)\n", g.Skipped)
	}
}
