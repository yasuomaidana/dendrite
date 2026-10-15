package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"Dendrite/pkg/audit"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/vcs"
)

func (s *Server) registerAuditTools(deps Dependencies) {
	// dendrite_audit_changes
	s.RegisterTool(Tool{
		Name:        "dendrite_audit_changes",
		Description: "Topological vendor audit and rebase engine for local/* stacked branches. Validates diffs, runs test suites, non-fast-forward merges into develop, and rebases downstream branches.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Optional target branch (e.g. 'local/vcs'). If omitted, audits all pending branches topologically.",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "Preview audit validation and rebase sequence without mutating branches.",
				},
				"max_tokens": map[string]interface{}{
					"type":        "integer",
					"description": "Max token context ceiling for audit payload.",
				},
				"model": map[string]interface{}{
					"type":        "string",
					"description": "Inference model for audit review (default: qwen3.8:27b).",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		targetBranch, _ := args["branch"].(string)
		dryRun, _ := args["dry_run"].(bool)
		maxTokens := 4096
		if mt, ok := args["max_tokens"].(float64); ok && mt > 0 {
			maxTokens = int(mt)
		}
		model, _ := args["model"].(string)
		if model == "" {
			model = "qwen3.8:27b"
		}

		guard := vcs.NewGitGuard(".", vcs.ModeAuditChanges)
		orch := vcs.NewStackedBranchOrchestrator(guard, "")
		gh := github.NewGitHubClient("yasuomaidana/dendrite")
		auditor := audit.NewAuditor(guard, orch, gh)

		summary, err := auditor.AuditChanges(ctx, audit.AuditOptions{
			TargetBranch: targetBranch,
			DryRun:       dryRun,
			MaxTokens:    maxTokens,
			Model:        model,
		})
		if err != nil {
			return nil, fmt.Errorf("audit changes failed: %w", err)
		}

		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// dendrite_github_sync
	s.RegisterTool(Tool{
		Name:        "dendrite_github_sync",
		Description: "Synchronize prioritized P0 tickets, dependency relations, and governance labels with remote GitHub Issues (yasuomaidana/dendrite).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"repo": map[string]interface{}{
					"type":        "string",
					"description": "Target GitHub repository in owner/repo format (default: yasuomaidana/dendrite)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		repo, _ := args["repo"].(string)
		if repo == "" {
			repo = "yasuomaidana/dendrite"
		}

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
			return nil, fmt.Errorf("github sync failed: %w", err)
		}

		data, err := json.MarshalIndent(synced, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// dendrite_vcs_guard
	s.RegisterTool(Tool{
		Name:        "dendrite_vcs_guard",
		Description: "Check or enforce Git branch guardrails (prohibits direct work on main/master, guarantees develop default).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"mode": map[string]interface{}{
					"type":        "string",
					"description": "Execution mode: 'STANDARD', 'DEGRADED_BUDGET', or 'AUDIT_CHANGES' (default: STANDARD)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		modeStr, _ := args["mode"].(string)
		mode := vcs.ModeStandard
		if modeStr != "" {
			mode = vcs.ExecutionMode(modeStr)
		}

		guard := vcs.NewGitGuard(".", mode)
		branch, err := guard.PreFlightBranchCheck(ctx)
		status := "OK"
		errMsg := ""
		if err != nil {
			status = "ALERT"
			errMsg = err.Error()
		}

		res := map[string]interface{}{
			"status":         status,
			"active_branch":  branch,
			"policy":         "forbidden on main/master; defaults to develop",
			"error":          errMsg,
			"execution_mode": string(mode),
		}

		data, _ := json.MarshalIndent(res, "", "  ")
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})
}
