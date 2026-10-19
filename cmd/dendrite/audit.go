package main

import (
	"context"
	"fmt"
	"io"

	"Dendrite/pkg/audit"
	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/vcs"

	"github.com/spf13/cobra"
)

// newPruneCmd builds `dendrite prune` (alias audit-changes): audit and merge the queued local/* branches.
func newPruneCmd() *cobra.Command {
	opts := audit.AuditOptions{MaxTokens: 4096, Model: "qwen3.8:27b"}
	cmd := &cobra.Command{
		Use:     "prune",
		Aliases: []string{"audit-changes", "microglial-prune", "synaptic-pruning", "prn"},
		Short:   "Topological vendor audit and rebase engine for local/* branches",
		GroupID: "glia",
		Args:    noArgs,
		Example: "dendrite prune --dry-run\ndendrite prune --branch local/module-a --max-tokens 8192",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.MaxTokens <= 0 {
				return usageErrorf("--max-tokens must be positive")
			}
			return runPrune(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVarP(&opts.TargetBranch, "branch", "b", "", "Audit only this branch (default: every pending local/* branch)")
	addDryRunFlag(cmd, &opts.DryRun)
	cmd.Flags().IntVar(&opts.MaxTokens, "max-tokens", opts.MaxTokens, "Token budget for the audit payload")
	addModelFlag(cmd, &opts.Model, opts.Model)
	return cmd
}

func runPrune(ctx context.Context, opts audit.AuditOptions, out io.Writer) error {
	fmt.Fprintln(out, "[+] Initializing Vendor LLM Topological Audit & Merge Engine...")
	if opts.TargetBranch != "" {
		fmt.Fprintf(out, "    - Target Branch: %s\n", opts.TargetBranch)
	} else {
		fmt.Fprintln(out, "    - Target Mode:   All Pending local/* Branches (Topological Order)")
	}
	if opts.DryRun {
		fmt.Fprintln(out, "    - Execution:     DRY-RUN (Simulated Validation)")
	}

	guard := vcs.NewGitGuard(".", vcs.ModeAuditChanges)
	// gh resolves the repository from the git remote.
	auditor := audit.NewAuditor(guard, vcs.NewStackedBranchOrchestrator(guard, ""), github.NewGitHubClient(""))
	summary, err := auditor.AuditChanges(ctx, opts)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, summary)
	}
	fmt.Fprintln(out, "[✓] Audit Process Completed:")
	fmt.Fprintf(out, "    - Branches Evaluated: %d\n", summary.BranchesAudited)
	fmt.Fprintf(out, "    - Branches Merged:    %d\n", summary.BranchesMerged)
	for _, res := range summary.Results {
		status := "MERGED"
		if !res.Merged {
			status = "SKIPPED/DRY-RUN"
		}
		fmt.Fprintf(out, "    * [%s] %s (Base: %s, Diff lines: %d, Verdict: %s)\n",
			status, res.Branch, res.BaseBranch, res.DiffLines, res.Verdict)
		if res.Notes != "" {
			fmt.Fprintf(out, "      Note: %s\n", res.Notes)
		}
	}
	return nil
}
