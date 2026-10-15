package plane

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	milestoneNumRegex = regexp.MustCompile(`(?i)Milestone\s+(\d+)`)
	ticketDenRegex    = regexp.MustCompile(`\[(?:DEN|DEND)-(\d+)\]`)
)

// GanttSyncReport summarizes the results of aligning milestones, tickets, and dependencies for Gantt charts.
type GanttSyncReport struct {
	TotalModules     int           `json:"total_modules"`
	UpdatedModules   int           `json:"updated_modules"`
	TotalTickets     int           `json:"total_tickets"`
	UpdatedTickets   int           `json:"updated_tickets"`
	RelationsCreated int           `json:"relations_created"`
	Duration         time.Duration `json:"duration"`
}

// ModuleSchedule holds chronological ordering and issues for a milestone module.
type ModuleSchedule struct {
	Module     PlaneModule
	Number     int
	StartDate  string
	TargetDate string
	Issues     []PlaneIssue
}

// SyncPlaneGanttAndRelations ensures all modules and tickets in Plane have valid, sequential,
// non-overlapping Gantt timeline dates and establishes dependency relation edges (blocked_by)
// within and across milestones so that Plane's Gantt charts render properly with visible dependency lines.
func SyncPlaneGanttAndRelations(ctx context.Context, client *Client) (*GanttSyncReport, error) {
	start := time.Now()

	modules, err := client.ListModules(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list Plane modules: %w", err)
	}

	issues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list Plane issues: %w", err)
	}

	report := &GanttSyncReport{
		TotalModules: len(modules),
		TotalTickets: len(issues),
	}

	if len(modules) == 0 && len(issues) == 0 {
		report.Duration = time.Since(start)
		return report, nil
	}

	// 1. Group issues by milestone/module assignment
	moduleIssueMap := make(map[string][]PlaneIssue)
	issueToModule := make(map[string]string)

	for _, mod := range modules {
		modIssues, err := client.ListModuleIssues(ctx, mod.ID)
		if err == nil && len(modIssues) > 0 {
			for _, mi := range modIssues {
				moduleIssueMap[mod.ID] = append(moduleIssueMap[mod.ID], mi)
				issueToModule[mi.ID] = mod.ID
			}
		}
	}

	// For any issues not explicitly attached to a module in Plane, infer from ticket sequence or module name
	for _, iss := range issues {
		if _, ok := issueToModule[iss.ID]; !ok {
			// Find best matching module
			var matchedModID string
			for _, mod := range modules {
				mNum := extractMilestoneNumber(mod.Name)
				if mNum > 0 && strings.Contains(strings.ToLower(iss.Name), strings.ToLower(fmt.Sprintf("Milestone %d", mNum))) {
					matchedModID = mod.ID
					break
				}
			}
			if matchedModID == "" && len(modules) > 0 {
				// Distribute by issue sequence
				idx := (iss.SequenceID - 1) % len(modules)
				if idx < 0 {
					idx = 0
				}
				matchedModID = modules[idx].ID
			}
			if matchedModID != "" {
				moduleIssueMap[matchedModID] = append(moduleIssueMap[matchedModID], iss)
				issueToModule[iss.ID] = matchedModID
			}
		}
	}

	// 2. Sort modules by milestone number
	var schedules []*ModuleSchedule
	for _, mod := range modules {
		schedules = append(schedules, &ModuleSchedule{
			Module: mod,
			Number: extractMilestoneNumber(mod.Name),
			Issues: moduleIssueMap[mod.ID],
		})
	}
	sort.Slice(schedules, func(i, j int) bool {
		if schedules[i].Number != schedules[j].Number {
			return schedules[i].Number < schedules[j].Number
		}
		return schedules[i].Module.Name < schedules[j].Module.Name
	})

	// 3. Compute baseline calendar start date
	baseDate := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	currentDate := baseDate

	var lastTicketUUIDOfPrevModule string

	for _, sched := range schedules {
		// Milestone duration based on ticket count (at least 5 days, 2 days per ticket)
		ticketCount := len(sched.Issues)
		if ticketCount == 0 {
			ticketCount = 1
		}
		durationDays := ticketCount * 3
		if durationDays < 5 {
			durationDays = 5
		}

		mStartStr := currentDate.Format("2006-01-02")
		mTargetDate := currentDate.AddDate(0, 0, durationDays)
		mTargetStr := mTargetDate.Format("2006-01-02")

		sched.StartDate = mStartStr
		sched.TargetDate = mTargetStr

		// Update module in Plane if dates missing or different
		needModUpdate := sched.Module.StartDate == nil || sched.Module.TargetDate == nil ||
			*sched.Module.StartDate == "" || *sched.Module.TargetDate == ""

		if needModUpdate {
			_, err := client.UpdateModule(ctx, sched.Module.ID, UpdateModuleRequest{
				StartDate:  &mStartStr,
				TargetDate: &mTargetStr,
			})
			if err == nil {
				report.UpdatedModules++
			}
		}

		// Sort issues within module by sequence ID
		sort.Slice(sched.Issues, func(i, j int) bool {
			return sched.Issues[i].SequenceID < sched.Issues[j].SequenceID
		})

		// Schedule tickets sequentially within the milestone window
		tDate := currentDate
		var prevTicketUUIDInMilestone string

		for _, iss := range sched.Issues {
			tStartStr := tDate.Format("2006-01-02")
			tTarget := tDate.AddDate(0, 0, 2)
			tTargetStr := tTarget.Format("2006-01-02")

			needTicketUpdate := iss.StartDate == nil || iss.TargetDate == nil ||
				*iss.StartDate == "" || *iss.TargetDate == ""

			if needTicketUpdate {
				prio := iss.Priority
				if prio == "" || prio == "none" {
					prio = "medium"
				}
				_, err := client.UpdatePlaneIssueWithDetails(ctx, iss.ID, UpdateIssueRequest{
					StartDate:  &tStartStr,
					TargetDate: &tTargetStr,
					Priority:   prio,
				})
				if err == nil {
					report.UpdatedTickets++
				}
			}

			// Establish dependency relations (blocked_by)
			if prevTicketUUIDInMilestone != "" {
				err := client.AddIssueRelation(ctx, iss.ID, "blocked_by", []string{prevTicketUUIDInMilestone})
				if err == nil {
					report.RelationsCreated++
				}
			} else if lastTicketUUIDOfPrevModule != "" {
				// Link milestone entry ticket to previous milestone's final ticket
				err := client.AddIssueRelation(ctx, iss.ID, "blocked_by", []string{lastTicketUUIDOfPrevModule})
				if err == nil {
					report.RelationsCreated++
				}
			}

			prevTicketUUIDInMilestone = iss.ID
			tDate = tDate.AddDate(0, 0, 2)
		}

		if prevTicketUUIDInMilestone != "" {
			lastTicketUUIDOfPrevModule = prevTicketUUIDInMilestone
		}

		// Advance timeline for next milestone
		currentDate = mTargetDate.AddDate(0, 0, 1)
	}

	report.Duration = time.Since(start)
	return report, nil
}

func extractMilestoneNumber(name string) int {
	matches := milestoneNumRegex.FindStringSubmatch(name)
	if len(matches) > 1 {
		if n, err := strconv.Atoi(matches[1]); err == nil {
			return n
		}
	}
	return 999
}
