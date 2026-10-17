package main

import (
	"context"
	"fmt"
	"strings"

	"Dendrite/pkg/devtool"
)

func handleCommitCommand(ctx context.Context, args []string) {
	stageAllSet := false
	opts := devtool.CommitOptions{
		StageAll:      false,
		UseLocalModel: true,
		Model:         "qwen3.8:27b",
		Smart:         true,
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--ticket", "-t":
			if i+1 < len(args) {
				opts.TicketID = args[i+1]
				i++
			}
		case "--message", "-m":
			if i+1 < len(args) {
				opts.Message = args[i+1]
				i++
			}
		case "--files", "-f":
			if i+1 < len(args) {
				opts.Files = strings.Split(args[i+1], ",")
				opts.StageAll = false
				i++
			}
		case "--all", "-a":
			opts.StageAll = true
			stageAllSet = true
		case "--amend":
			opts.Amend = true
		case "--smart":
			opts.Smart = true
			opts.NoSmart = false
		case "--no-smart":
			opts.Smart = false
			opts.NoSmart = true
		case "--model":
			if i+1 < len(args) {
				opts.Model = args[i+1]
				i++
			}
		case "--local":
			opts.UseLocalModel = true
		case "--no-local":
			opts.UseLocalModel = false
		case "--dry-run":
			opts.DryRun = true
		case "--allow-mocks", "--no-mock-gate":
			opts.AllowMocks = true
		case "--help", "-h":
			printCommitUsage()
			return
		}
	}

	// If user did not specify files or --all, and nothing is staged yet, default to staging all
	if !stageAllSet && len(opts.Files) == 0 {
		// devtool.ExecuteCommit will intelligently check if index already has staged files
		opts.StageAll = false
	}

	fmt.Println("[+] Initializing Autonomous Git Commit Engine...")
	if opts.TicketID != "" {
		fmt.Printf("    - Active Ticket:    %s\n", opts.TicketID)
	} else if !opts.NoSmart {
		fmt.Println("    - Smart Strategy:   Auto-resolving active ticket and context from Plane / DAG...")
	}
	if opts.Amend {
		fmt.Println("    - Commit Mode:      AMEND (--amend)")
	}
	if opts.UseLocalModel && opts.Message == "" {
		fmt.Printf("    - Message Model:    %s (Delegated Local Engine)\n", opts.Model)
	} else if opts.Message != "" {
		fmt.Printf("    - Message Source:   User Specified\n")
	}
	if opts.DryRun {
		fmt.Println("    - Execution Mode:   DRY-RUN (Preview Only)")
	}

	res, err := devtool.ExecuteCommit(ctx, opts)
	if err != nil {
		fmt.Printf("[-] Commit failed: %v\n", err)
		return
	}

	if res.DryRun {
		fmt.Println("[✓] Dry run commit preview generated:")
		if res.SmartTicket != nil && res.SmartTicket.ID != "" {
			fmt.Printf("    - Smart Strategy:   Resolved [%s] via %s\n", res.SmartTicket.ID, res.SmartSource)
			if res.SmartTicket.Title != "" {
				fmt.Printf("      * Title:          %s\n", res.SmartTicket.Title)
			}
		}
		fmt.Printf("    - Proposed Message:\n%s\n", indentText(res.Message, "      "))
		fmt.Printf("    - Staged Files (%d):\n", len(res.Files))
		for _, f := range res.Files {
			fmt.Printf("      * %s\n", f)
		}
		fmt.Printf("    - Model Engine:     %s (Cost: $%.2f)\n", res.ModelUsed, res.CostUSD)
		return
	}

	fmt.Printf("[✓] Commit applied successfully (%s)\n", res.CommitSHA[:min(8, len(res.CommitSHA))])
	if res.SmartTicket != nil && res.SmartTicket.ID != "" {
		fmt.Printf("    - Smart Strategy:   Resolved [%s] via %s\n", res.SmartTicket.ID, res.SmartSource)
		if res.SmartTicket.Title != "" {
			fmt.Printf("      * Title:          %s\n", res.SmartTicket.Title)
		}
	}
	fmt.Printf("    - Message:\n%s\n", indentText(res.Message, "      "))
	fmt.Printf("    - Files Committed (%d):\n", len(res.Files))
	for _, f := range res.Files {
		fmt.Printf("      * %s\n", f)
	}
	fmt.Printf("    - Model Provenance: %s (Cost: $%.2f)\n", res.ModelUsed, res.CostUSD)
}

func printCommitUsage() {
	fmt.Println("Usage: dendrite commit [flags]")
	fmt.Println("\nFlags:")
	fmt.Println("  --ticket, -t <id>     Target Plane Ticket ID (e.g. DEN-098)")
	fmt.Println("  --message, -m <msg>   Explicit commit message (skips local model generation)")
	fmt.Println("  --files, -f <f1,f2>   Comma-separated files to stage (defaults to git add -A)")
	fmt.Println("  --all, -a             Stage all modified and untracked files (default true)")
	fmt.Println("  --amend               Amend previous commit instead of creating a new commit")
	fmt.Println("  --smart / --no-smart  Enable/disable smart ticket auto-detection (default enabled)")
	fmt.Println("  --model <model>       Target local model for message synthesis (default qwen3.8:27b)")
	fmt.Println("  --local / --no-local  Enable/disable local model delegation (default enabled)")
	fmt.Println("  --dry-run             Preview staged diff and synthesized commit message without committing")
}

func indentText(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = indent + l
	}
	return strings.Join(lines, "\n")
}
