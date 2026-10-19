package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"Dendrite/pkg/quality"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/github"
)

func newMyelinGitHubSyncCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:     "github-sync",
		Short:   "Synchronize prioritized tickets and poll HITL verification state on GitHub Issues",
		Args:    noArgs,
		Example: "dendrite myelin github-sync --repo yasuomaidana/dendrite",
		RunE: func(c *cobra.Command, args []string) error {
			return runMyelinGitHubSync(c.Context(), repo, c.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "yasuomaidana/dendrite", "Target GitHub repository (owner/repo)")
	return cmd
}

func runMyelinGitHubSync(ctx context.Context, repo string, out io.Writer) error {
	if repo == "" {
		repo = "yasuomaidana/dendrite"
	}

	fmt.Fprintf(out, "[+] Synchronizing Sequenced P0 Blockers to GitHub Issues (%s)...\n", repo)
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

	var errs []error
	synced, err := client.SyncTickets(ctx, tickets)
	if err != nil {
		errs = append(errs, fmt.Errorf("github synchronization failed: %w", err))
	} else {
		fmt.Fprintln(out, "[✓] Successfully synchronized tickets to GitHub:")
		for _, t := range synced {
			fmt.Fprintf(out, "    * #%d: [%s] %s (Module: %s, Labels: P0-TOP-BLOCKER, %s)\n",
				t.GitHubIssueNum, t.ID, t.Title, t.Module, t.Difficulty.String())
		}
	}

	fmt.Fprintln(out, "[+] Polling GitHub Issues for Human-in-the-Loop Verification...")
	poller := quality.NewGitHubHITLPoller(repo, &github.DefaultCommandExecutor{})
	hitlTickets := []*quality.IssueSpec{
		{ID: "DEN-701", GitHubIssueNum: 1, State: quality.StateToVerify},
		{ID: "DEN-702", GitHubIssueNum: 2, State: quality.StateToVerify},
		{ID: "DEN-703", GitHubIssueNum: 3, State: quality.StateToVerify},
		{ID: "DEN-704", GitHubIssueNum: 4, State: quality.StateToVerify},
		{ID: "DEN-705", GitHubIssueNum: 5, State: quality.StateToVerify},
	}

	for _, t := range hitlTickets {
		followUp, err := poller.SyncValidationState(ctx, t)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to sync %s: %w", t.ID, err))
			continue
		}
		if followUp != nil {
			fmt.Fprintf(out, "[!] Human feedback received on %s! Demoted to TO_DO. Follow-up: %s\n", t.ID, followUp.ID)
		} else {
			fmt.Fprintf(out, "    * [%s] Current State: %s (GitHub #%d)\n", t.ID, t.State, t.GitHubIssueNum)
		}
	}

	return errors.Join(errs...)
}

func handleGitHubSyncCommand(ctx context.Context, args []string) {
	_ = invokeCmd(ctx, newMyelinCmd(), append([]string{"github-sync"}, args...)...)
}
