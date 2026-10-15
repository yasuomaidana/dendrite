package main

import (
	"context"
	"fmt"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/github"
)

func handleGitHubSyncCommand(ctx context.Context, args []string) {
	repo := "yasuomaidana/dendrite"
	for i := 0; i < len(args); i++ {
		if (args[i] == "--repo" || args[i] == "-r") && i+1 < len(args) {
			repo = args[i+1]
			i++
		}
	}

	fmt.Printf("[+] Synchronizing Sequenced P0 Blockers to GitHub Issues (%s)...\n", repo)
	client := github.NewGitHubClient(repo)

	tickets := []*tracker.RoutingTicket{
		{
			ID:            "DEN-701",
			Title:         "Git Branch Guard: main lock, develop default & atomic CommitAndPush",
			Description:   "Halts direct main commits with ErrForbiddenBranchMain, evacuates to develop automatically, and enforces atomic CommitAndPush transactions with auto-rebase on divergence.",
			Priority:      "P0_TOP_BLOCKER",
			Difficulty:    tracker.TierL2Low,
			LocalFeasible: true,
			RequiresAudit: true,
			Module:        "vcs",
			Blocks:        []string{"DEN-702"},
		},
		{
			ID:            "DEN-702",
			Title:         "Ticket Domain Model, Triage & Low-Budget DAG Prioritizer",
			Description:   "Enriches tickets with DifficultyTier (L1-L5), InferenceEngine, and AuditStatus; provides EnrichTicketClassification, SelectInferenceEngine, and SortForLowBudgetQueue.",
			Priority:      "P0_TOP_BLOCKER",
			Difficulty:    tracker.TierL2Low,
			LocalFeasible: true,
			RequiresAudit: true,
			Module:        "tracker",
			DependsOn:     []string{"DEN-701"},
			Blocks:        []string{"DEN-703"},
		},
		{
			ID:            "DEN-703",
			Title:         "GitHub Issues Two-Way Synchronizer & Label Provisioner",
			Description:   "Syncs prioritized tickets with remote GitHub Issues (yasuomaidana/dendrite), auto-provisions labels (P0-TOP-BLOCKER, local-llm-delegated, needs-vendor-audit, module:<name>), and links dependencies.",
			Priority:      "P0_TOP_BLOCKER",
			Difficulty:    tracker.TierL3Medium,
			LocalFeasible: true,
			RequiresAudit: true,
			Module:        "tracker",
			DependsOn:     []string{"DEN-702"},
			Blocks:        []string{"DEN-704"},
		},
		{
			ID:            "DEN-704",
			Title:         "Speculative Stacked Branch Orchestrator & Degraded Budget Mode",
			Description:   "Switches to DegradedBudgetMode under low token budgets, locks develop merges with ErrMergeLockedInDegradedMode, creates local/<module> stacked branches, and records audit manifests in .dendrite/audit_queue.json.",
			Priority:      "P0_TOP_BLOCKER",
			Difficulty:    tracker.TierL3Medium,
			LocalFeasible: true,
			RequiresAudit: true,
			Module:        "vcs",
			DependsOn:     []string{"DEN-703"},
			Blocks:        []string{"DEN-705"},
		},
		{
			ID:            "DEN-705",
			Title:         "dendrite audit-changes CLI Command & Vendor LLM Rebase/Merge Engine",
			Description:   "Topologically audits queued local/* branches, constructs token-efficient diff payloads, applies fixes, non-fast-forward merges into develop, and rebases downstream branches.",
			Priority:      "P0_TOP_BLOCKER",
			Difficulty:    tracker.TierL4High,
			LocalFeasible: false,
			RequiresAudit: false,
			Module:        "audit",
			DependsOn:     []string{"DEN-704"},
		},
	}

	synced, err := client.SyncTickets(ctx, tickets)
	if err != nil {
		fmt.Printf("[-] GitHub synchronization failed: %v\n", err)
		return
	}

	fmt.Println("[✓] Successfully synchronized tickets to GitHub:")
	for _, t := range synced {
		fmt.Printf("    * #%d: [%s] %s (Module: %s, Labels: P0-TOP-BLOCKER, %s)\n",
			t.GitHubIssueNum, t.ID, t.Title, t.Module, t.Difficulty.String())
	}
}
