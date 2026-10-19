package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"Dendrite/pkg/audit"
	"Dendrite/pkg/quality"
	"Dendrite/pkg/tracker/github"
	"Dendrite/pkg/tracker/hitl"
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
		gh := github.NewGitHubClient("") // gh resolves the repository from the git remote
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
		Description: "Mirror Plane verification state onto GitHub issues: open a HITL verification issue for every To Verify ticket, add missing labels, and close issues of tickets Done in Plane. Tickets come from Plane and the repository from the git origin remote.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"repo": map[string]interface{}{
					"type":        "string",
					"description": "Target GitHub repository in owner/repo format (default: the origin remote of the working directory)",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "Plan the GitHub changes without applying them",
				},
				"tickets": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Limit the sync to these ticket IDs (default: every ticket)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		repo, _ := args["repo"].(string)
		dryRun, _ := args["dry_run"].(bool)
		syncer, err := newHITLSyncer(ctx, deps, repo)
		if err != nil {
			return nil, fmt.Errorf("github sync failed: %w", err)
		}
		if ids, ok := args["tickets"].([]interface{}); ok {
			for _, id := range ids {
				if str, ok := id.(string); ok {
					syncer.Only = append(syncer.Only, str)
				}
			}
		}
		res, err := syncer.GitHubSync(ctx, dryRun)
		if err != nil {
			return nil, fmt.Errorf("github sync failed: %w", err)
		}
		return jsonResult(res)
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

	// dendrite_remote_sync
	s.RegisterTool(Tool{
		Name:        "dendrite_remote_sync",
		Description: "Fetch and detect remote commits/hotfixes, pull with rebase, and reconcile tickets.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Branch to sync (default: active branch)",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		branch, _ := args["branch"].(string)
		guard := vcs.NewGitGuard(".", vcs.ModeStandard)
		watcher := vcs.NewRemoteSyncWatcher(guard, nil)
		status, err := watcher.SyncRemoteFixes(ctx, branch)
		if err != nil {
			return nil, fmt.Errorf("remote sync failed: %w", err)
		}
		data, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// dendrite_quality_audit
	s.RegisterTool(Tool{
		Name:        "dendrite_quality_audit",
		Description: "Execute anti-mock static scan and 100% coverage profiler against a package.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pkg": map[string]interface{}{
					"type":        "string",
					"description": "Package path to audit (e.g. pkg/vcs)",
				},
			},
			"required": []interface{}{"pkg"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		pkgPath, _ := args["pkg"].(string)
		if pkgPath == "" {
			return nil, fmt.Errorf("missing pkg parameter")
		}
		spec := &quality.IssueSpec{
			ID:                 "AUDIT-" + pkgPath,
			Title:              "Package Audit: " + pkgPath,
			State:              quality.StateToVerify,
			CodebaseLocation:   pkgPath,
			ReproductionSteps:  "go test -race ./" + pkgPath + "/...",
			ProposedResolution: "Ensure zero mocks and 100% coverage",
			AcceptanceCriteria: []string{"Zero mocks", "100% statement coverage"},
		}
		gaps, err := quality.AuditFinishedTicket(ctx, ".", spec, pkgPath, nil, nil)
		if err != nil {
			return nil, err
		}
		out := map[string]interface{}{
			"pkg":          pkgPath,
			"ticket_state": spec.State,
			"rejection":    spec.ProposedResolution,
			"gap_tickets":  gaps,
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// dendrite_hitl_sync
	s.RegisterTool(Tool{
		Name:        "dendrite_hitl_sync",
		Description: "Poll the GitHub verification issue of every Plane To Verify ticket: issues closed as completed move the ticket to Done, reviewer comments are reported as feedback.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"repo": map[string]interface{}{
					"type":        "string",
					"description": "Target GitHub repository in owner/repo format (default: the origin remote of the working directory)",
				},
				"dry_run": map[string]interface{}{
					"type":        "boolean",
					"description": "Report approvals without moving tickets to Done",
				},
			},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		repo, _ := args["repo"].(string)
		dryRun, _ := args["dry_run"].(bool)
		syncer, err := newHITLSyncer(ctx, deps, repo)
		if err != nil {
			return nil, fmt.Errorf("hitl sync failed: %w", err)
		}
		results, err := syncer.HITLSync(ctx, dryRun)
		if err != nil {
			return nil, fmt.Errorf("hitl sync failed: %w", err)
		}
		if results == nil {
			results = []hitl.HITLResult{}
		}
		return jsonResult(results)
	})
}

// newHITLSyncer builds the Plane/GitHub verification syncer, reusing the server's
// Plane client when one is configured. It is a seam so tests stay offline.
var newHITLSyncer = func(ctx context.Context, deps Dependencies, repo string) (*hitl.Syncer, error) {
	if deps.PlaneClient == nil {
		return hitl.NewFromEnv(ctx, repo, ".")
	}
	if repo == "" {
		detected, err := github.DetectRepo(ctx, &github.DefaultCommandExecutor{}, ".")
		if err != nil {
			return nil, err
		}
		repo = detected
	}
	return hitl.New(deps.PlaneClient, github.NewGitHubClient(repo), "."), nil
}
