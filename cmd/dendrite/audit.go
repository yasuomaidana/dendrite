package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"Dendrite/pkg/audit"
	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/vcs"
)

type pruneOptions struct {
	Branch    string
	DryRun    bool
	MaxTokens int
	Model     string
	Repo      string
}

// newPruneCmd builds `dendrite prune`: topological vendor audit and merge engine for local/* branches.
func newPruneCmd() *cobra.Command {
	var o pruneOptions
	cmd := &cobra.Command{
		Use:     "prune",
		Aliases: []string{"audit-changes", "microglial-prune", "synaptic-pruning", "prn"},
		Short:   "Topological vendor audit and rebase engine for local/* branches",
		GroupID: "glia",
		Args:    noArgs,
		Example: "dendrite prune --dry-run\ndendrite prune --branch local/vcs --max-tokens 8192",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrune(cmd.Context(), o, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.Branch, "branch", "b", "", "Target specific branch (e.g. local/module-a)")
	addDryRunFlag(cmd, &o.DryRun)
	f.IntVar(&o.MaxTokens, "max-tokens", 4096, "Ceiling token budget for audit payload context")
	addModelFlag(cmd, &o.Model, "qwen3.8:27b")
	f.StringVar(&o.Repo, "repo", "yasuomaidana/dendrite", "Target GitHub repository (owner/repo)")
	return cmd
}

func runPrune(ctx context.Context, o pruneOptions, out io.Writer) error {
	opts := audit.AuditOptions{
		TargetBranch: o.Branch,
		DryRun:       o.DryRun,
		MaxTokens:    o.MaxTokens,
		Model:        o.Model,
	}

	fmt.Fprintln(out, "[+] Initializing Vendor LLM Topological Audit & Merge Engine...")
	if opts.TargetBranch != "" {
		fmt.Fprintf(out, "    - Target Branch: %s\n", opts.TargetBranch)
	} else {
		fmt.Fprintln(out, "    - Target Mode:   All Pending local/* Branches (Topological Order)")
	}
	if opts.DryRun {
		fmt.Fprintln(out, "    - Execution:     DRY-RUN (Simulated Validation)")
	}

	repo := o.Repo
	if repo == "" {
		repo = "yasuomaidana/dendrite"
	}

	guard := vcs.NewGitGuard(".", vcs.ModeAuditChanges)
	orch := vcs.NewStackedBranchOrchestrator(guard, "")
	gh := github.NewGitHubClient(repo)
	auditor := audit.NewAuditor(guard, orch, gh)

	summary, err := auditor.AuditChanges(ctx, opts)
	if err != nil {
		return fmt.Errorf("audit execution failed: %w", err)
	}

	fmt.Fprintf(out, "[✓] Audit Process Completed:\n")
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

func handleAuditChangesCommand(ctx context.Context, args []string) {
	_ = invokeCmd(ctx, newPruneCmd(), args...)
}
