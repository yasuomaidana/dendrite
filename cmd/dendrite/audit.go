package main

import (
	"context"
	"fmt"
	"strconv"

	"Dendrite/pkg/audit"
	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/vcs"
)

func handleAuditChangesCommand(ctx context.Context, args []string) {
	opts := audit.AuditOptions{
		MaxTokens: 4096,
		Model:     "qwen3.8:27b",
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--branch", "-b":
			if i+1 < len(args) {
				opts.TargetBranch = args[i+1]
				i++
			}
		case "--dry-run":
			opts.DryRun = true
		case "--max-tokens":
			if i+1 < len(args) {
				if val, err := strconv.Atoi(args[i+1]); err == nil {
					opts.MaxTokens = val
				}
				i++
			}
		case "--model":
			if i+1 < len(args) {
				opts.Model = args[i+1]
				i++
			}
		case "--help", "-h":
			printAuditUsage()
			return
		}
	}

	fmt.Println("[+] Initializing Vendor LLM Topological Audit & Merge Engine...")
	if opts.TargetBranch != "" {
		fmt.Printf("    - Target Branch: %s\n", opts.TargetBranch)
	} else {
		fmt.Println("    - Target Mode:   All Pending local/* Branches (Topological Order)")
	}
	if opts.DryRun {
		fmt.Println("    - Execution:     DRY-RUN (Simulated Validation)")
	}

	guard := vcs.NewGitGuard(".", vcs.ModeAuditChanges)
	orch := vcs.NewStackedBranchOrchestrator(guard, "")
	gh := github.NewGitHubClient("") // gh resolves the repository from the git remote
	auditor := audit.NewAuditor(guard, orch, gh)

	summary, err := auditor.AuditChanges(ctx, opts)
	if err != nil {
		fmt.Printf("[-] Audit execution failed: %v\n", err)
		return
	}

	fmt.Printf("[✓] Audit Process Completed:\n")
	fmt.Printf("    - Branches Evaluated: %d\n", summary.BranchesAudited)
	fmt.Printf("    - Branches Merged:    %d\n", summary.BranchesMerged)
	for _, res := range summary.Results {
		status := "MERGED"
		if !res.Merged {
			status = "SKIPPED/DRY-RUN"
		}
		fmt.Printf("    * [%s] %s (Base: %s, Diff lines: %d, Verdict: %s)\n",
			status, res.Branch, res.BaseBranch, res.DiffLines, res.Verdict)
		if res.Notes != "" {
			fmt.Printf("      Note: %s\n", res.Notes)
		}
	}
}

func printAuditUsage() {
	fmt.Println("Usage: dendrite audit-changes [flags]")
	fmt.Println("\nFlags:")
	fmt.Println("  --branch, -b <name>   Target specific branch (e.g. local/module-a)")
	fmt.Println("  --dry-run             Preview topological rebase and audit checks without merging")
	fmt.Println("  --max-tokens <int>    Ceiling token budget for audit payload context (default 4096)")
	fmt.Println("  --model <name>        Inference model for audit validation (default qwen3.8:27b)")
}
