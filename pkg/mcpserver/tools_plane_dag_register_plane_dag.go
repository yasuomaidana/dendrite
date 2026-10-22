package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"Dendrite/pkg/tracker/dag"
	"Dendrite/pkg/tracker/plane"
)

func registerPlaneDAGTools(s *Server, deps Dependencies) {
	handleSmartSelect := planeSmartSelectHandler(deps)

	// 1. dendrite_ticket_smart_select
	s.RegisterTool(Tool{
		Name:        "dendrite_ticket_smart_select",
		Description: "Smart selects the optimal unblocked ticket to be done based on priority (Urgent > High > Medium > Low), dependency relationships (DAG unblocked check), and downstream unblock impact (critical path weight). If ticket_id is specified by user, evaluates its blockers and selects it.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket_id": map[string]interface{}{
					"type":        "string",
					"description": "Optional specific ticket ID requested by user (e.g. 'DEN-036')",
				},
				"milestone": map[string]interface{}{
					"type":        "string",
					"description": "Optional milestone name or filter",
				},
				"file": map[string]interface{}{
					"type":        "string",
					"description": "Optional fallback source markdown file (default: DEV_STATUS.md)",
				},
			},
		},
	}, handleSmartSelect)

	// 2. dendrite_plane_ticket_smart_select (alias)
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_ticket_smart_select",
		Description: "Smart selects the optimal unblocked ticket from Plane v2 project management container respecting priority and blocker relationships.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket_id": map[string]interface{}{
					"type":        "string",
					"description": "Optional specific ticket ID requested by user (e.g. 'DEN-036')",
				},
				"milestone": map[string]interface{}{
					"type":        "string",
					"description": "Optional milestone filter",
				},
			},
		},
	}, handleSmartSelect)

	// 3. dendrite_plane_relation_link
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_relation_link",
		Description: "Create a dependency relationship between tickets in Plane (relation_type: 'blocking', 'blocked_by', 'relates_to', 'duplicate') for Gantt diagrams and DAG scheduling",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"issue_id": map[string]interface{}{
					"type":        "string",
					"description": "Source ticket ID or UUID (e.g. 'DEN-036' or issue UUID)",
				},
				"relation_type": map[string]interface{}{
					"type":        "string",
					"description": "Relation type: 'blocking', 'blocked_by', 'relates_to', or 'duplicate'",
				},
				"target_issue_id": map[string]interface{}{
					"type":        "string",
					"description": "Target ticket ID or UUID that is related/blocked",
				},
			},
			"required": []interface{}{"issue_id", "relation_type", "target_issue_id"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		issueID, _ := args["issue_id"].(string)
		relationType, _ := args["relation_type"].(string)
		targetIssueID, _ := args["target_issue_id"].(string)

		if issueID == "" || relationType == "" || targetIssueID == "" {
			return nil, fmt.Errorf("issue_id, relation_type, and target_issue_id are all required")
		}

		client := deps.PlaneClient
		if client == nil {
			var err error
			client, err = plane.NewClientFromEnv()
			if err != nil {
				return nil, fmt.Errorf("plane client error: %w", err)
			}
		}

		// Resolve ticket ID prefixes (e.g. DEN-036) to UUIDs if needed
		resolveUUID := func(raw string) (string, error) {
			if len(raw) == 36 && strings.Count(raw, "-") == 4 {
				return raw, nil
			}
			issues, err := client.ListPlaneIssues(ctx)
			if err != nil {
				return "", err
			}
			for _, iss := range issues {
				if strings.Contains(iss.Name, "["+raw+"]") || strings.Contains(iss.Name, raw) {
					return iss.ID, nil
				}
			}
			return "", fmt.Errorf("ticket '%s' not found in Plane", raw)
		}

		sourceUUID, err := resolveUUID(issueID)
		if err != nil {
			return nil, err
		}
		targetUUID, err := resolveUUID(targetIssueID)
		if err != nil {
			return nil, err
		}

		if err := client.AddIssueRelation(ctx, sourceUUID, relationType, []string{targetUUID}); err != nil {
			return nil, fmt.Errorf("failed to create relation in Plane: %w", err)
		}

		res := map[string]interface{}{
			"status":          "success",
			"relation_type":   relationType,
			"source_ticket":   issueID,
			"source_uuid":     sourceUUID,
			"target_ticket":   targetIssueID,
			"target_uuid":     targetUUID,
			"gantt_connected": true,
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})

	// 4. dendrite_plane_issue_create
	s.RegisterTool(Tool{
		Name:        "dendrite_plane_issue_create",
		Description: "Create a new ticket in Plane, assign it to a milestone/module, set priority, schedule start and target dates for the Gantt timeline, and establish blocker relationships to place it correctly in the development plan",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Ticket title / name (e.g. '[DEN-101] Feature Name')",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Mandatory ticket description (HTML or markdown detailing goals, acceptance criteria, and DoD)",
				},
				"context": map[string]interface{}{
					"type":        "string",
					"description": "Alias for description",
				},
				"milestone": map[string]interface{}{
					"type":        "string",
					"description": "Milestone / Module name or UUID to assign this ticket to",
				},
				"priority": map[string]interface{}{
					"type":        "string",
					"description": "Priority: 'urgent', 'high', 'medium', 'low', or 'none' (default: 'medium')",
				},
				"estimate": map[string]interface{}{
					"type":        "integer",
					"description": "Story points estimate (e.g. 3, 5, 8, 13)",
				},
				"start_date": map[string]interface{}{
					"type":        "string",
					"description": "Optional start date in YYYY-MM-DD format (auto-calculated if omitted)",
				},
				"target_date": map[string]interface{}{
					"type":        "string",
					"description": "Optional target date in YYYY-MM-DD format (auto-calculated if omitted)",
				},
				"blocked_by": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "List of ticket IDs or UUIDs that block this ticket",
				},
			},
			"required": []interface{}{"name", "description"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		name, _ := args["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("missing required 'name' parameter")
		}
		desc, _ := args["description"].(string)
		if desc == "" {
			desc, _ = args["context"].(string)
		}
		if plane.IsEmptyContext(desc) {
			return nil, fmt.Errorf("ticket context/description cannot be empty: acceptance criteria and architectural context are mandatory")
		}
		mName, _ := args["milestone"].(string)
		prio, _ := args["priority"].(string)
		if prio == "" {
			prio = dag.PriorityMedium
		}

		var estimate *int
		if estFloat, ok := args["estimate"].(float64); ok {
			e := int(estFloat)
			estimate = &e
		}

		sDate, _ := args["start_date"].(string)
		tDate, _ := args["target_date"].(string)

		client := deps.PlaneClient
		if client == nil {
			var err error
			client, err = plane.NewClientFromEnv()
			if err != nil {
				return nil, fmt.Errorf("plane client error: %w", err)
			}
		}

		// Auto-calculate dates if omitted
		if sDate == "" || tDate == "" {
			now := time.Now().Truncate(24 * time.Hour)
			days := 4
			if estimate != nil {
				days = dag.EstimateDays(*estimate, 0)
			}
			if sDate == "" {
				sDate = now.Format("2006-01-02")
			}
			if tDate == "" {
				tDate = now.AddDate(0, 0, days).Format("2006-01-02")
			}
		}

		// Resolve or create module
		var moduleID string
		if mName != "" {
			modules, err := client.ListModules(ctx)
			if err == nil {
				for _, mod := range modules {
					if strings.Contains(strings.ToLower(mod.Name), strings.ToLower(mName)) || mod.ID == mName {
						moduleID = mod.ID
						break
					}
				}
			}
			if moduleID == "" {
				newMod, err := client.CreateModuleWithDates(ctx, mName, "in-progress", fmt.Sprintf("Milestone tracking for %s", mName), &sDate, &tDate)
				if err == nil && newMod != nil {
					moduleID = newMod.ID
				}
			}
		}

		// Resolve blocker tickets
		var blockerUUIDs []string
		if rawBlockers, ok := args["blocked_by"].([]interface{}); ok {
			existingIssues, _ := client.ListPlaneIssues(ctx)
			for _, rb := range rawBlockers {
				bStr, ok := rb.(string)
				if !ok || bStr == "" {
					continue
				}
				if len(bStr) == 36 && strings.Count(bStr, "-") == 4 {
					blockerUUIDs = append(blockerUUIDs, bStr)
				} else {
					for _, iss := range existingIssues {
						if strings.Contains(iss.Name, "["+bStr+"]") || strings.Contains(iss.Name, bStr) {
							blockerUUIDs = append(blockerUUIDs, iss.ID)
							break
						}
					}
				}
			}
		}

		descHTML := fmt.Sprintf("<div><p>%s</p></div>", desc)
		created, err := client.CreatePlaneIssueWithDetails(ctx, plane.CreateIssueRequest{
			Name:            name,
			DescriptionHTML: descHTML,
			Priority:        prio,
			EstimatePoint:   estimate,
			StartDate:       &sDate,
			TargetDate:      &tDate,
			ModuleID:        moduleID,
			BlockedBy:       blockerUUIDs,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create issue in Plane: %w", err)
		}

		res := map[string]interface{}{
			"status":            "success",
			"issue_id":          created.ID,
			"sequence_id":       created.SequenceID,
			"name":              created.Name,
			"priority":          prio,
			"start_date":        sDate,
			"target_date":       tDate,
			"module_id":         moduleID,
			"blockers_count":    len(blockerUUIDs),
			"gantt_plan_placed": true,
		}

		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	})
}
