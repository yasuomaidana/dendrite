package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/dag"
	"Dendrite/pkg/tracker/plane"
)

var ticketNumRegex = regexp.MustCompile(`\[?(DEN-\d+)\]?`)

func (s *Server) registerPlaneDAGTools(deps Dependencies) {
	handleSmartSelect := func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		ticketID, _ := args["ticket_id"].(string)
		milestoneFilter, _ := args["milestone"].(string)
		sourceFile := deps.DevStatus
		if f, ok := args["file"].(string); ok && f != "" {
			sourceFile = f
		}

		client := deps.PlaneClient
		if client == nil {
			var err error
			client, err = plane.NewClientFromEnv()
			if err != nil {
				client = nil
			}
		}

		graph := dag.NewGraph()

		// 1. Try querying authoritative Plane service if reachable
		planeLoaded := false
		if client != nil {
			issues, err := client.ListPlaneIssues(ctx)
			if err == nil && len(issues) > 0 {
				states, _ := client.ListStates(ctx)
				stateMap := make(map[string]tracker.Status)
				for _, st := range states {
					g := strings.ToLower(st.Group)
					n := strings.ToLower(st.Name)
					switch {
					case g == "completed" || strings.Contains(n, "done"):
						stateMap[st.ID] = tracker.StatusDone
					case strings.Contains(n, "review") || strings.Contains(n, "verify"):
						stateMap[st.ID] = tracker.StatusInReview
					case g == "started" || strings.Contains(n, "progress") || strings.Contains(n, "active"):
						stateMap[st.ID] = tracker.StatusInProgress
					case g == "unstarted" || strings.Contains(n, "todo") || strings.Contains(n, "to do"):
						stateMap[st.ID] = tracker.StatusTodo
					default:
						stateMap[st.ID] = tracker.StatusBacklog
					}
				}

				uuidToID := make(map[string]string)
				for _, iss := range issues {
					id := fmt.Sprintf("DEN-%03d", iss.SequenceID)
					m := ticketNumRegex.FindStringSubmatch(iss.Name)
					if len(m) > 1 {
						id = m[1]
					}
					uuidToID[iss.ID] = id

					st := stateMap[iss.State]
					if st == "" {
						st = tracker.StatusTodo
					}

					prio := iss.Priority
					if prio == "" || prio == "none" {
						prio = dag.PriorityMedium
					}

					est := 5
					if iss.EstimatePoint != nil {
						est = *iss.EstimatePoint
					}

					sDate := ""
					if iss.StartDate != nil {
						sDate = *iss.StartDate
					}
					tDate := ""
					if iss.TargetDate != nil {
						tDate = *iss.TargetDate
					}

					graph.AddNode(&dag.Node{
						ID:             id,
						SequenceID:     iss.SequenceID,
						Title:          iss.Name,
						Status:         st,
						Priority:       prio,
						EstimatePoints: est,
						StartDate:      sDate,
						TargetDate:     tDate,
					})
				}

				// Fetch relations from Plane
				for _, iss := range issues {
					currID := uuidToID[iss.ID]
					rels, err := client.GetIssueRelations(ctx, iss.ID)
					if err == nil && rels != nil {
						for _, blocked := range rels.BlockedBy {
							blockerID := uuidToID[blocked.ID]
							if blockerID == "" {
								m := ticketNumRegex.FindStringSubmatch(blocked.Name)
								if len(m) > 1 {
									blockerID = m[1]
								}
							}
							if blockerID != "" && blockerID != currID {
								_ = graph.AddEdge(blockerID, currID)
							}
						}
					}
				}

				planeLoaded = true
			}
		}

		// 2. Fallback to local markdown ledger if Plane unavailable or empty
		if !planeLoaded {
			content, err := os.ReadFile(sourceFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read %s and Plane is not available: %w", sourceFile, err)
			}
			data, err := plane.ParseDevStatus(string(content))
			if err != nil {
				return nil, fmt.Errorf("failed to parse dev status markdown: %w", err)
			}

			for _, t := range data.Tickets {
				if milestoneFilter != "" && !strings.Contains(strings.ToLower(t.Milestone), strings.ToLower(milestoneFilter)) {
					continue
				}

				sp := 5
				if t.StoryPoints != "" {
					var parsedSP int
					if n, _ := fmt.Sscanf(t.StoryPoints, "%d", &parsedSP); n > 0 {
						sp = parsedSP
					}
				}
				var comp float64 = 5.0
				if t.ComplexityScore != "" {
					var parsedComp float64
					if n, _ := fmt.Sscanf(t.ComplexityScore, "%f", &parsedComp); n > 0 {
						comp = parsedComp
					}
				}

				prio := dag.PriorityMedium
				switch {
				case comp >= 9.0:
					prio = dag.PriorityUrgent
				case comp >= 7.0:
					prio = dag.PriorityHigh
				case comp >= 4.0:
					prio = dag.PriorityMedium
				default:
					prio = dag.PriorityLow
				}
				if strings.Contains(data.ActiveMilestone, t.Milestone) && prio == dag.PriorityMedium {
					prio = dag.PriorityHigh
				}

				graph.AddNode(&dag.Node{
					ID:             t.ID,
					SequenceID:     t.Number,
					Title:          t.Title,
					Milestone:      t.Milestone,
					Story:          t.FeatureID,
					Status:         t.Status,
					Priority:       prio,
					EstimatePoints: sp,
					Complexity:     comp,
				})
			}

			// Add sequential edges within milestones
			mMap := make(map[string][]string)
			for _, t := range data.Tickets {
				mMap[t.Milestone] = append(mMap[t.Milestone], t.ID)
			}
			for _, tList := range mMap {
				for i := 1; i < len(tList); i++ {
					_ = graph.AddEdge(tList[i-1], tList[i])
				}
			}
		}

		// Ensure Gantt plan is generated
		_, _ = graph.GenerateGanttPlan(time.Now().Truncate(24 * time.Hour))

		res, err := graph.SmartSelect(ticketID)
		if err != nil {
			return nil, fmt.Errorf("smart selection failed: %w", err)
		}

		output := map[string]interface{}{
			"status":            "success",
			"authoritative_src": map[string]interface{}{"plane_online": planeLoaded},
			"selection":         res,
		}

		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return nil, err
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil
	}

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
					"description": "Optional description (HTML or markdown)",
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
			"required": []interface{}{"name"},
		},
	}, func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		name, _ := args["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("missing required 'name' parameter")
		}
		desc, _ := args["description"].(string)
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
