package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/dag"
	"Dendrite/pkg/tracker/plane"
)

// planeSmartSelectHandler answers dendrite_ticket_smart_select and its Plane alias.
func planeSmartSelectHandler(deps Dependencies) func(context.Context, map[string]interface{}) (*CallToolResult, error) {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
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
					stateMap[st.ID] = plane.StatusFor(st.Group, st.Name)
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

				// Fetch relations from Plane concurrently; finished tickets cannot be blocked by anything
				// that matters for selection, so they are skipped.
				var relIDs []string
				for _, iss := range issues {
					if st := stateMap[iss.State]; st != tracker.StatusDone && st != tracker.StatusCancelled {
						relIDs = append(relIDs, iss.ID)
					}
				}
				relations, _ := client.GetRelationsFor(ctx, relIDs)
				for _, iss := range issues {
					currID := uuidToID[iss.ID]
					rels := relations[iss.ID]
					if rels != nil {
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
}
