package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"Dendrite/pkg/devtool"

	"github.com/spf13/cobra"
)

// newQuantalCmd builds `dendrite quantal` (alias commit): ticket-aware commits with generated messages, then push.
func newQuantalCmd() *cobra.Command {
	opts := devtool.CommitOptions{UseLocalModel: true, Model: "qwen3.8:27b", Smart: true}
	var noSmart, noLocal bool
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
			"dendrite quantal --files cmd/a.go,cmd/b.go --dry-run",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Smart, opts.NoSmart = !noSmart, noSmart
			opts.UseLocalModel = !noLocal
			if strings.EqualFold(os.Getenv("DENDRITE_COMMIT_PUSH"), "false") {
				opts.NoPush = true
			}
			return runQuantal(cmd.Context(), opts, cmd.OutOrStdout())
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
	cmd.MarkFlagsMutuallyExclusive("smart", "no-smart")
	cmd.MarkFlagsMutuallyExclusive("local", "no-local")
	cmd.MarkFlagsMutuallyExclusive("files", "all")
	return cmd
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

	if res.DryRun {
		fmt.Fprintln(out, "[✓] Dry run commit preview generated:")
		if res.SmartTicket != nil && res.SmartTicket.ID != "" {
			fmt.Fprintf(out, "    - Smart Strategy:   Resolved [%s] via %s\n", res.SmartTicket.ID, res.SmartSource)
			if res.SmartTicket.Title != "" {
				fmt.Fprintf(out, "      * Title:          %s\n", res.SmartTicket.Title)
			}
		}
		fmt.Fprintf(out, "    - Proposed Message:\n%s\n", indentText(res.Message, "      "))
		fmt.Fprintf(out, "    - Staged Files (%d):\n", len(res.Files))
		for _, f := range res.Files {
			fmt.Fprintf(out, "      * %s\n", f)
		}
		fmt.Fprintf(out, "    - Model Engine:     %s (Cost: $%.2f)\n", res.ModelUsed, res.CostUSD)
		if res.PushSkipped {
			fmt.Fprintf(out, "    - Remote Push:      Skipped (%s)\n", res.PushReason)
		}
		return nil
	}

	fmt.Fprintf(out, "[✓] Commit applied successfully (%s)\n", res.CommitSHA[:min(8, len(res.CommitSHA))])
	if res.SmartTicket != nil && res.SmartTicket.ID != "" {
		fmt.Fprintf(out, "    - Smart Strategy:   Resolved [%s] via %s\n", res.SmartTicket.ID, res.SmartSource)
		if res.SmartTicket.Title != "" {
			fmt.Fprintf(out, "      * Title:          %s\n", res.SmartTicket.Title)
		}
	}
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
		return fmt.Errorf("git push failed: %w", res.PushError)
	}
	return nil
}

func indentText(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = indent + l
	}
	return strings.Join(lines, "\n")
}
