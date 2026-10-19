package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"Dendrite/pkg/quality"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/github"

	"github.com/spf13/cobra"
)

const defaultGitHubRepo = "yasuomaidana/dendrite"

// newMyelinGitHubSyncCmd builds `myelin github-sync`: push the P0 blocker chain to GitHub Issues,
// and with --hitl poll those issues for human verification.
func newMyelinGitHubSyncCmd() *cobra.Command {
	var repo string
	var hitl bool
	cmd := &cobra.Command{
		Use:     "github-sync",
		Aliases: []string{"sync-github", "saltatory-sync", "gsync"},
		Short:   "Sync the P0 blocker tickets and their dependencies to GitHub Issues (--hitl polls for human verification)",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runGitHubSync(cmd.Context(), repo, cmd.OutOrStdout()); err != nil {
				return err
			}
			if !hitl {
				return nil
			}
			return runHITLSync(cmd.Context(), repo, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVarP(&repo, "repo", "r", defaultGitHubRepo, "GitHub repository (owner/name)")
	cmd.Flags().BoolVar(&hitl, "hitl", false, "Then poll the issues for human-in-the-loop approval or feedback")
	return cmd
}

// p0Blockers is the sequenced P0 blocker chain mirrored to GitHub.
func p0Blockers() []*tracker.RoutingTicket {
	return []*tracker.RoutingTicket{
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

}

func runGitHubSync(ctx context.Context, repo string, out io.Writer) error {
	fmt.Fprintf(out, "[+] Synchronizing Sequenced P0 Blockers to GitHub Issues (%s)...\n", repo)
	synced, err := github.NewGitHubClient(repo).SyncTickets(ctx, p0Blockers())
	if err != nil {
		return fmt.Errorf("GitHub synchronization: %w", err)
	}
	fmt.Fprintln(out, "[✓] Successfully synchronized tickets to GitHub:")
	for _, t := range synced {
		fmt.Fprintf(out, "    * #%d: [%s] %s (Module: %s, Labels: P0-TOP-BLOCKER, %s)\n",
			t.GitHubIssueNum, t.ID, t.Title, t.Module, t.Difficulty.String())
	}
	return nil
}

// runHITLSync polls the P0 issues: closed approvals advance, new feedback demotes the ticket.
func runHITLSync(ctx context.Context, repo string, out io.Writer) error {
	fmt.Fprintln(out, "[+] Polling GitHub Issues for Human-in-the-Loop Verification...")
	poller := quality.NewGitHubHITLPoller(repo, &github.DefaultCommandExecutor{})
	var errs []error
	for i, b := range p0Blockers() {
		t := &quality.IssueSpec{ID: b.ID, GitHubIssueNum: i + 1, State: quality.StateToVerify}
		followUp, err := poller.SyncValidationState(ctx, t)
		if err != nil {
			errs = append(errs, fmt.Errorf("sync %s: %w", t.ID, err))
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
