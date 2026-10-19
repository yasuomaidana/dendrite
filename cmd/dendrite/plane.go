package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/dag"
	"Dendrite/pkg/tracker/plane"

	"github.com/spf13/cobra"
)

var cliTicketPrefixRegex = regexp.MustCompile(`\[?(DEN-\d+)\]?`)

// newPlaneClient is a seam so tests can stub the Plane client.
var newPlaneClient = plane.NewClientFromEnv

// devStatusFile is the Markdown mirror of the Plane board.
const devStatusFile = "DEV_STATUS.md"

// relationTypes are the issue relations Plane accepts.
var relationTypes = []string{"blocking", "blocked_by", "relates_to", "duplicate", "start_before", "start_after", "finish_before", "finish_after"}

// planeRun opens the Plane client and runs fn with it.
func planeRun(fn func(*plane.Client) error) error {
	client, err := newPlaneClient()
	if err != nil {
		return fmt.Errorf("initialize Plane client: %w", err)
	}
	return fn(client)
}

// newChronosCmd builds `dendrite chronos` (alias plane): Plane issue tracking, Gantt alignment and DAG scheduling.
func newChronosCmd() *cobra.Command {
	cmd := newGroupCmd("chronos", "Sovereign issue tracking, Gantt alignment, and DAG scheduler", "boutons",
		"plane", "chronos-tracker", "pln")
	cmd.Example = "dendrite chronos next --json\n" +
		"dendrite chronos create --name \"[DEN-123] Title\" --desc-file spec.md --blocked-by DEN-100\n" +
		"dendrite chronos link DEN-123 relates_to DEN-100\n" +
		"dendrite chronos ticket set DEN-123 done\n" +
		"dendrite chronos replicate --dry-run"
	cmd.AddCommand(
		newChronosNextCmd(),
		newChronosLinkCmd(),
		newChronosRelateCmd(),
		newChronosGanttSyncCmd(),
		newChronosCreateCmd(),
		newChronosUpdateCmd(),
		newChronosIssuesCmd(),
		newChronosModulesCmd(),
		newChronosPagesCmd(),
		newChronosReplicateCmd(),
		newChronosPushCmd(),
		newChronosStatesCmd(),
		newChronosTicketCmd(),
	)
	return cmd
}

func newChronosReplicateCmd() *cobra.Command {
	var file string
	var dryRun bool
	cmd := &cobra.Command{
		Use:     "replicate",
		Aliases: []string{"pull", "status", "pull-status"},
		Short:   "Rewrite DEV_STATUS.md from the Plane board",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				return runChronosReplicate(cmd.Context(), c, file, dryRun, cmd.OutOrStdout())
			})
		},
	}
	addFileFlag(cmd, &file, devStatusFile, "Status file to write")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func runChronosReplicate(ctx context.Context, client *plane.Client, file string, dryRun bool, out io.Writer) error {
	fmt.Fprintf(out, "[+] Replicating DEV_STATUS from Plane service (%s)...\n", client.BaseURL())
	rendered, report, err := plane.ReplicateStatusFromPlane(ctx, client)
	if err != nil {
		return fmt.Errorf("replicate from Plane: %w", err)
	}
	if dryRun {
		fmt.Fprintln(out, "[+] Dry run mode: simulated replication output:")
		fmt.Fprintln(out, rendered)
	} else {
		if err := os.WriteFile(file, []byte(rendered), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", file, err)
		}
		fmt.Fprintf(out, "[✓] Successfully replicated %s from Plane (Ownership: Plane Service)\n", file)
	}
	fmt.Fprintf(out, "    - Total Tickets:      %d\n", report.TotalTickets)
	fmt.Fprintf(out, "    - Completed:          %d (%.1f%%)\n", report.CompletedTickets, report.OverallProgress)
	fmt.Fprintf(out, "    - In Progress:        %d\n", report.InProgressTickets)
	fmt.Fprintf(out, "    - Queued / To Do:     %d\n", report.TodoTickets)
	fmt.Fprintf(out, "    - Active Milestone:   %s\n", report.ActiveMilestone)
	fmt.Fprintf(out, "    - Current Focus:      %s\n", report.CurrentFocus)
	fmt.Fprintf(out, "    - Sync Duration:      %v\n", report.Duration)
	return nil
}

func newChronosPushCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:     "push",
		Aliases: []string{"push-status"},
		Short:   "Push DEV_STATUS.md to Plane with Gantt timelines and dependencies",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				return runChronosPush(cmd.Context(), c, file, cmd.OutOrStdout())
			})
		},
	}
	addFileFlag(cmd, &file, devStatusFile, "Status file to read")
	return cmd
}

func runChronosPush(ctx context.Context, client *plane.Client, file string, out io.Writer) error {
	fmt.Fprintf(out, "[+] Moving %s content to Plane service (%s) with Gantt timelines & dependencies...\n", file, client.BaseURL())
	content, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}
	report, err := plane.PushStatusToPlane(ctx, client, string(content))
	if err != nil {
		return fmt.Errorf("push dev status to Plane: %w", err)
	}
	fmt.Fprintf(out, "[✓] Completed moving DEV_STATUS to Plane in %v\n", report.Duration)
	fmt.Fprintf(out, "    - Tickets Synchronized: %d\n", report.TotalTickets)
	fmt.Fprintf(out, "    - Completed Tickets:    %d\n", report.CompletedTickets)
	fmt.Fprintf(out, "    - In Progress Tickets:  %d\n", report.InProgressTickets)
	fmt.Fprintf(out, "    - Milestones (Modules): %d\n", report.ModulesCount)
	fmt.Fprintf(out, "    - Gantt Dependencies:   %d\n", report.RelationsCount)
	fmt.Fprintf(out, "    - Roadmap Pages:        %d\n", report.PagesCount)
	fmt.Fprintf(out, "    - Overall Progress:     %.1f%%\n", report.OverallProgress)
	return nil
}

func newChronosNextCmd() *cobra.Command {
	var ticket, milestone string
	cmd := &cobra.Command{
		Use:     "next",
		Short:   "Pick the next ticket by priority and DAG reachability",
		Args:    noArgs,
		Example: "dendrite chronos next\ndendrite chronos next --ticket DEN-123 --json\ndendrite chronos next --milestone M3",
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				return runChronosNext(cmd.Context(), c, ticket, milestone, cmd.OutOrStdout())
			})
		},
	}
	addTicketFlag(cmd, &ticket, "")
	cmd.Flags().StringVarP(&milestone, "milestone", "m", "", "Only consider tickets whose name contains this milestone")
	return cmd
}

// planeStatusFor maps a Plane state to a tracker status by group and name.
func planeStatusFor(st plane.PlaneState) tracker.Status {
	g := strings.ToLower(st.Group)
	n := strings.ToLower(st.Name)
	switch {
	case g == "completed" || strings.Contains(n, "done"):
		return tracker.StatusDone
	case strings.Contains(n, "review") || strings.Contains(n, "verify"):
		return tracker.StatusInReview
	case g == "started" || strings.Contains(n, "progress") || strings.Contains(n, "active"):
		return tracker.StatusInProgress
	case g == "unstarted" || strings.Contains(n, "todo") || strings.Contains(n, "to do"):
		return tracker.StatusTodo
	default:
		return tracker.StatusBacklog
	}
}

// buildPlaneDAG loads the Plane board into a dependency graph.
func buildPlaneDAG(ctx context.Context, client *plane.Client, milestone string) (*dag.Graph, error) {
	issues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		return nil, fmt.Errorf("query Plane issues: %w", err)
	}
	states, _ := client.ListStates(ctx)
	stateMap := make(map[string]tracker.Status, len(states))
	for _, st := range states {
		stateMap[st.ID] = planeStatusFor(st)
	}

	graph := dag.NewGraph()
	uuidToID := make(map[string]string)
	for _, iss := range issues {
		if milestone != "" && !strings.Contains(strings.ToLower(iss.Name), strings.ToLower(milestone)) {
			continue
		}
		id := fmt.Sprintf("DEN-%03d", iss.SequenceID)
		if m := cliTicketPrefixRegex.FindStringSubmatch(iss.Name); len(m) > 1 {
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
		node := &dag.Node{
			ID:             id,
			SequenceID:     iss.SequenceID,
			Title:          iss.Name,
			Status:         st,
			Priority:       prio,
			EstimatePoints: est,
		}
		if iss.StartDate != nil {
			node.StartDate = *iss.StartDate
		}
		if iss.TargetDate != nil {
			node.TargetDate = *iss.TargetDate
		}
		graph.AddNode(node)
	}

	for _, iss := range issues {
		currID := uuidToID[iss.ID]
		rels, err := client.GetIssueRelations(ctx, iss.ID)
		if err != nil || rels == nil {
			continue
		}
		for _, blocked := range rels.BlockedBy {
			bID := uuidToID[blocked.ID]
			if bID == "" {
				if m := cliTicketPrefixRegex.FindStringSubmatch(blocked.Name); len(m) > 1 {
					bID = m[1]
				}
			}
			if bID != "" && bID != currID {
				_ = graph.AddEdge(bID, currID)
			}
		}
	}
	_, _ = graph.GenerateGanttPlan(time.Now().Truncate(24 * time.Hour))
	return graph, nil
}

func runChronosNext(ctx context.Context, client *plane.Client, ticket, milestone string, out io.Writer) error {
	graph, err := buildPlaneDAG(ctx, client, milestone)
	if err != nil {
		return err
	}
	res, err := graph.SmartSelect(ticket)
	if err != nil {
		return fmt.Errorf("smart selection: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, res)
	}

	fmt.Fprintln(out, "[+] Smart Ticket Prioritization & DAG Reachability (Source: Plane v2)")
	t := res.SelectedTicket
	fmt.Fprintf(out, "    * Selected Ticket:    %s\n", t.ID)
	fmt.Fprintf(out, "    * Title:              %s\n", t.Title)
	fmt.Fprintf(out, "    * Priority:           %s (Base Score: %.0f, Total Score: %.1f)\n",
		strings.ToUpper(t.Priority), t.PriorityScore, res.TotalScore)
	fmt.Fprintf(out, "    * Operational Status: %s\n", t.Status)
	if res.IsActive {
		fmt.Fprintln(out, "    * Active State:       ALREADY IN PROGRESS")
	}
	if res.IsBlocked {
		fmt.Fprintf(out, "    * Blocker Warning:    BLOCKED by unresolved prerequisites: %s\n", strings.Join(res.BlockingTickets, ", "))
	} else {
		fmt.Fprintln(out, "    * Dependency Status:  UNBLOCKED (all prerequisites satisfied)")
	}
	if len(res.UnblockedDownstream) > 0 {
		fmt.Fprintf(out, "    * Downstream Impact:  Unblocks %d dependent tickets: %s\n",
			len(res.UnblockedDownstream), strings.Join(res.UnblockedDownstream, ", "))
	}
	if t.StartDate != "" && t.TargetDate != "" {
		fmt.Fprintf(out, "    * Gantt Schedule:     %s -> %s\n", t.StartDate, t.TargetDate)
	}
	fmt.Fprintf(out, "    * Decision Reason:    %s\n", res.Reason)
	return nil
}

func newChronosLinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "link <ticket-a> <relation> <ticket-b>",
		Short:     "Add one relation between two tickets (" + strings.Join(relationTypes, ", ") + ")",
		Args:      exactArgs(3),
		ValidArgs: relationTypes,
		Example:   "dendrite chronos link DEN-123 blocked_by DEN-100\ndendrite chronos link DEN-123 relates_to DEN-950",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !slices.Contains(relationTypes, args[1]) {
				return usageErrorf("unknown relation %q: use %s", args[1], strings.Join(relationTypes, ", "))
			}
			return planeRun(func(c *plane.Client) error {
				return runChronosLink(cmd.Context(), c, args[0], args[1], args[2], cmd.OutOrStdout())
			})
		},
	}
}

func runChronosLink(ctx context.Context, client *plane.Client, a, rel, b string, out io.Writer) error {
	uA, err := resolvePlaneTicketUUID(ctx, client, a)
	if err != nil {
		return err
	}
	uB, err := resolvePlaneTicketUUID(ctx, client, b)
	if err != nil {
		return err
	}
	if err := client.AddIssueRelation(ctx, uA, rel, []string{uB}); err != nil {
		return fmt.Errorf("link issues in Plane: %w", err)
	}
	fmt.Fprintf(out, "[✓] Successfully linked %s -> %s (%s) for Plane Gantt dependency view\n", a, b, rel)
	return nil
}

type chronosRelateOptions struct {
	Ticket     string
	Priority   string
	BlockedBy  []string
	Blocking   []string
	RelatesTo  []string
	Reason     string
	StartDate  string
	TargetDate string
}

func newChronosRelateCmd() *cobra.Command {
	var o chronosRelateOptions
	cmd := &cobra.Command{
		Use:     "relate",
		Short:   "Set a ticket's priority and dates and add several relations at once",
		Args:    noArgs,
		Example: "dendrite chronos relate --ticket DEN-123 --priority high --blocked-by DEN-100,DEN-101 --relates-to DEN-950",
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Priority = strings.ToLower(o.Priority)
			return planeRun(func(c *plane.Client) error {
				return runChronosRelate(cmd.Context(), c, o, cmd.OutOrStdout())
			})
		},
	}
	addTicketFlag(cmd, &o.Ticket, "")
	requireFlags(cmd, "ticket")
	f := cmd.Flags()
	f.StringVarP(&o.Priority, "priority", "p", "", "New priority (urgent, high, medium, low, none)")
	f.StringSliceVarP(&o.BlockedBy, "blocked-by", "b", nil, "Tickets that block this one (comma-separated)")
	f.StringSliceVar(&o.Blocking, "blocking", nil, "Tickets this one blocks (comma-separated)")
	f.StringSliceVarP(&o.RelatesTo, "relates-to", "r", nil, "Related tickets (comma-separated)")
	f.StringVar(&o.Reason, "reason", "", "Why the relations exist (echoed for the record)")
	f.StringVarP(&o.StartDate, "start", "s", "", "Start date (YYYY-MM-DD)")
	f.StringVar(&o.TargetDate, "target", "", "Target date (YYYY-MM-DD)")
	return cmd
}

func runChronosRelate(ctx context.Context, client *plane.Client, o chronosRelateOptions, out io.Writer) error {
	uTarget, err := resolvePlaneTicketUUID(ctx, client, o.Ticket)
	if err != nil {
		return fmt.Errorf("resolve ticket %s: %w", o.Ticket, err)
	}

	var errs []error
	if o.Priority != "" || o.StartDate != "" || o.TargetDate != "" {
		req := plane.UpdateIssueRequest{Priority: o.Priority}
		if o.StartDate != "" {
			req.StartDate = &o.StartDate
		}
		if o.TargetDate != "" {
			req.TargetDate = &o.TargetDate
		}
		if _, err := client.UpdatePlaneIssueWithDetails(ctx, uTarget, req); err != nil {
			errs = append(errs, fmt.Errorf("update ticket metadata: %w", err))
		} else {
			fmt.Fprintf(out, "[✓] Updated ticket %s metadata (Priority: %s)\n", o.Ticket, strings.ToUpper(o.Priority))
		}
	}

	for _, group := range []struct {
		rel string
		ids []string
	}{{"blocked_by", o.BlockedBy}, {"blocking", o.Blocking}, {"relates_to", o.RelatesTo}} {
		for _, id := range group.ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			u, err := resolvePlaneTicketUUID(ctx, client, id)
			if err == nil {
				err = client.AddIssueRelation(ctx, uTarget, group.rel, []string{u})
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("link %s %s: %w", group.rel, id, err))
				continue
			}
			fmt.Fprintf(out, "[✓] Linked: %s %s %s\n", o.Ticket, group.rel, id)
		}
	}

	if o.Reason != "" {
		fmt.Fprintf(out, "    * Stored context: %s\n", o.Reason)
	}
	return errors.Join(errs...)
}

func newChronosGanttSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gantt-sync",
		Short: "Align milestone dates, ticket schedules and dependency arrows on the Plane Gantt view",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				return runChronosGanttSync(cmd.Context(), c, cmd.OutOrStdout())
			})
		},
	}
}

func runChronosGanttSync(ctx context.Context, client *plane.Client, out io.Writer) error {
	fmt.Fprintln(out, "[+] Synchronizing Plane Gantt timelines, milestone dates, and dependency arrows...")
	rep, err := plane.SyncPlaneGanttAndRelations(ctx, client)
	if err != nil {
		return fmt.Errorf("gantt synchronization: %w", err)
	}
	fmt.Fprintf(out, "[✓] Plane Gantt schedule & dependency relations synchronized in %v:\n", rep.Duration)
	fmt.Fprintf(out, "    - Milestones Scheduled:  %d (Total: %d)\n", rep.UpdatedModules, rep.TotalModules)
	fmt.Fprintf(out, "    - Tickets Scheduled:     %d (Total: %d)\n", rep.UpdatedTickets, rep.TotalTickets)
	fmt.Fprintf(out, "    - Dependency Arrows:     %d\n", rep.RelationsCreated)
	fmt.Fprintf(out, "    - Live Gantt View URL:   %s/dendrite-ws/projects/%s/issues/ (Layout: Gantt)\n", client.BaseURL(), client.ProjectID())
	return nil
}

// descriptionFrom returns the --desc value, or the contents of --desc-file when given.
func descriptionFrom(desc, descFile string) (string, error) {
	if descFile == "" {
		return desc, nil
	}
	data, err := os.ReadFile(descFile)
	if err != nil {
		return "", fmt.Errorf("read description file: %w", err)
	}
	return string(data), nil
}

func addDescFlags(cmd *cobra.Command, desc, descFile *string) {
	cmd.Flags().StringVarP(desc, "desc", "d", "", "Markdown description (context and acceptance criteria)")
	cmd.Flags().StringVarP(descFile, "desc-file", "f", "", "Read the Markdown description from a file")
	cmd.MarkFlagsMutuallyExclusive("desc", "desc-file")
}

type chronosCreateOptions struct {
	Name      string
	Desc      string
	DescFile  string
	Milestone string
	Priority  string
	Estimate  int
	BlockedBy []string
}

func newChronosCreateCmd() *cobra.Command {
	var o chronosCreateOptions
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a ticket, place it on the Gantt plan and link its blockers",
		Args:  noArgs,
		Example: "dendrite chronos create --name \"[DEN-123] Title\" --desc-file spec.md\n" +
			"dendrite chronos create --name \"[DEN-124] Title\" --desc \"Goal and acceptance criteria\" --milestone M3 --priority high --estimate 5 --blocked-by DEN-100,DEN-101",
		RunE: func(cmd *cobra.Command, args []string) error {
			desc, err := descriptionFrom(o.Desc, o.DescFile)
			if err != nil {
				return err
			}
			if strings.TrimSpace(o.Name) == "" {
				return usageErrorf("--name must not be empty")
			}
			if plane.IsEmptyContext(desc) {
				return usageErrorf("a ticket needs a description: give --desc or --desc-file with the goal and acceptance criteria")
			}
			o.Desc = desc
			var estimate *int
			if cmd.Flags().Changed("estimate") {
				estimate = &o.Estimate
			}
			return planeRun(func(c *plane.Client) error {
				return runChronosCreate(cmd.Context(), c, o, estimate, cmd.OutOrStdout())
			})
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.Name, "name", "n", "", "Ticket title, e.g. \"[DEN-123] Title\"")
	addDescFlags(cmd, &o.Desc, &o.DescFile)
	f.StringVarP(&o.Milestone, "milestone", "m", "", "Milestone (Plane module) name or ID; created when missing")
	f.StringVarP(&o.Priority, "priority", "p", "medium", "Priority (urgent, high, medium, low, none)")
	f.IntVarP(&o.Estimate, "estimate", "e", 0, "Estimate in points; sets the Gantt duration")
	f.StringSliceVarP(&o.BlockedBy, "blocked-by", "b", nil, "Blocking tickets or issue UUIDs (comma-separated)")
	requireFlags(cmd, "name")
	return cmd
}

func isPlaneUUID(s string) bool { return len(s) == 36 && strings.Count(s, "-") == 4 }

func runChronosCreate(ctx context.Context, client *plane.Client, o chronosCreateOptions, estimate *int, out io.Writer) error {
	now := time.Now().Truncate(24 * time.Hour)
	days := 4
	if estimate != nil {
		days = dag.EstimateDays(*estimate, 0)
	}
	sDate := now.Format("2006-01-02")
	tDate := now.AddDate(0, 0, days).Format("2006-01-02")

	var moduleID string
	if o.Milestone != "" {
		if modules, err := client.ListModules(ctx); err == nil {
			for _, mod := range modules {
				if strings.Contains(strings.ToLower(mod.Name), strings.ToLower(o.Milestone)) || mod.ID == o.Milestone {
					moduleID = mod.ID
					break
				}
			}
		}
		if moduleID == "" {
			newMod, err := client.CreateModuleWithDates(ctx, o.Milestone, "in-progress", fmt.Sprintf("Milestone tracking for %s", o.Milestone), &sDate, &tDate)
			if err == nil && newMod != nil {
				moduleID = newMod.ID
			}
		}
	}

	var blockerUUIDs []string
	if len(o.BlockedBy) > 0 {
		existing, _ := client.ListPlaneIssues(ctx)
		for _, b := range o.BlockedBy {
			b = strings.TrimSpace(b)
			if b == "" {
				continue
			}
			if isPlaneUUID(b) {
				blockerUUIDs = append(blockerUUIDs, b)
				continue
			}
			for _, iss := range existing {
				if strings.Contains(iss.Name, "["+b+"]") || strings.Contains(iss.Name, b) {
					blockerUUIDs = append(blockerUUIDs, iss.ID)
					break
				}
			}
		}
	}

	created, err := client.CreatePlaneIssueWithDetails(ctx, plane.CreateIssueRequest{
		Name:            o.Name,
		DescriptionHTML: plane.MarkdownToHTML(o.Desc),
		Priority:        o.Priority,
		EstimatePoint:   estimate,
		StartDate:       &sDate,
		TargetDate:      &tDate,
		ModuleID:        moduleID,
		BlockedBy:       blockerUUIDs,
	})
	if err != nil {
		return fmt.Errorf("create issue in Plane: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, created)
	}

	fmt.Fprintln(out, "[✓] Successfully created ticket in Plane & placed on Gantt development plan:")
	fmt.Fprintf(out, "    - ID:             %s (#%d)\n", created.ID, created.SequenceID)
	fmt.Fprintf(out, "    - Name:           %s\n", created.Name)
	fmt.Fprintf(out, "    - Priority:       %s\n", strings.ToUpper(o.Priority))
	fmt.Fprintf(out, "    - Gantt Timeline: %s -> %s\n", sDate, tDate)
	if moduleID != "" {
		fmt.Fprintf(out, "    - Milestone:      %s\n", o.Milestone)
	}
	if len(blockerUUIDs) > 0 {
		fmt.Fprintf(out, "    - Dependencies:   Blocked by %d tickets\n", len(blockerUUIDs))
	}
	return nil
}

func newChronosUpdateCmd() *cobra.Command {
	var ticket, name, desc, descFile string
	cmd := &cobra.Command{
		Use:     "update",
		Short:   "Rename a ticket or replace its description",
		Args:    noArgs,
		Example: "dendrite chronos update --ticket DEN-123 --desc-file spec.md\ndendrite chronos update -t DEN-123 --name \"[DEN-123] New title\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := descriptionFrom(desc, descFile)
			if err != nil {
				return err
			}
			if name == "" && plane.IsEmptyContext(d) {
				return usageErrorf("nothing to update: give --name, --desc or --desc-file")
			}
			return planeRun(func(c *plane.Client) error {
				return runChronosUpdate(cmd.Context(), c, ticket, name, d, cmd.OutOrStdout())
			})
		},
	}
	addTicketFlag(cmd, &ticket, "")
	requireFlags(cmd, "ticket")
	cmd.Flags().StringVarP(&name, "name", "n", "", "New ticket title")
	addDescFlags(cmd, &desc, &descFile)
	return cmd
}

func runChronosUpdate(ctx context.Context, client *plane.Client, ticket, name, desc string, out io.Writer) error {
	issues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		return fmt.Errorf("list Plane issues: %w", err)
	}
	var target *plane.PlaneIssue
	for i := range issues {
		if strings.Contains(issues[i].Name, "["+ticket+"]") {
			target = &issues[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("ticket %s not found in Plane", ticket)
	}

	req := plane.UpdateIssueRequest{Name: name}
	if !plane.IsEmptyContext(desc) {
		req.DescriptionHTML = plane.MarkdownToHTML(desc)
	}
	updated, err := client.UpdatePlaneIssueWithDetails(ctx, target.ID, req)
	if err != nil {
		return fmt.Errorf("update issue in Plane: %w", err)
	}
	fmt.Fprintf(out, "[✓] Updated ticket %s (#%d): %s\n", ticket, updated.SequenceID, updated.Name)
	return nil
}

func newChronosIssuesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "issues",
		Aliases: []string{"list", "ls"},
		Short:   "List the project's Plane issues",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				return runChronosIssues(cmd.Context(), c, cmd.OutOrStdout())
			})
		},
	}
}

func runChronosIssues(ctx context.Context, client *plane.Client, out io.Writer) error {
	issues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		return fmt.Errorf("list Plane issues: %w", err)
	}
	if globals.JSON {
		if issues == nil {
			issues = []plane.PlaneIssue{}
		}
		return writeJSON(out, issues)
	}
	fmt.Fprintf(out, "[+] Plane Issues (%d) for Project '%s':\n", len(issues), client.ProjectID())
	for _, iss := range issues {
		st := iss.StateDetail.Name
		if st == "" {
			st = iss.State
		}
		prio := iss.Priority
		if prio == "" {
			prio = "none"
		}
		dateSpan := ""
		if iss.StartDate != nil && iss.TargetDate != nil {
			dateSpan = fmt.Sprintf(" [%s -> %s]", *iss.StartDate, *iss.TargetDate)
		}
		fmt.Fprintf(out, "    [%s | %s] #%d: %s%s\n", st, strings.ToUpper(prio), iss.SequenceID, iss.Name, dateSpan)
	}
	return nil
}

func newChronosModulesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "modules",
		Aliases: []string{"milestones"},
		Short:   "List Plane modules (milestones)",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				return runChronosModules(cmd.Context(), c, cmd.OutOrStdout())
			})
		},
	}
}

func runChronosModules(ctx context.Context, client *plane.Client, out io.Writer) error {
	modules, err := client.ListModules(ctx)
	if err != nil {
		return fmt.Errorf("list Plane modules: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, modules)
	}
	fmt.Fprintf(out, "[+] Plane Modules / Milestones (%d):\n", len(modules))
	for _, mod := range modules {
		dateSpan := ""
		if mod.StartDate != nil && mod.TargetDate != nil {
			dateSpan = fmt.Sprintf(" (Dates: %s -> %s)", *mod.StartDate, *mod.TargetDate)
		}
		fmt.Fprintf(out, "    - %s (Status: %s, ID: %s)%s\n", mod.Name, mod.Status, mod.ID, dateSpan)
	}
	return nil
}

func newChronosPagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pages",
		Short: "List Plane documentation pages",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				return runChronosPages(cmd.Context(), c, cmd.OutOrStdout())
			})
		},
	}
}

func runChronosPages(ctx context.Context, client *plane.Client, out io.Writer) error {
	pages, err := client.ListPages(ctx)
	if err != nil {
		return fmt.Errorf("list Plane pages: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, pages)
	}
	fmt.Fprintf(out, "[+] Plane Documentation Pages (%d):\n", len(pages))
	for _, p := range pages {
		fmt.Fprintf(out, "    - %s (ID: %s, Updated: %s)\n", p.Name, p.ID, p.UpdatedAt.Format("2006-01-02 15:04"))
	}
	return nil
}

func newChronosStatesCmd() *cobra.Command {
	cmd := newGroupCmd("states", "Manage the project's workflow states", "")
	var project string
	ensure := &cobra.Command{
		Use:   "ensure",
		Short: "Create any missing canonical lifecycle states",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeRun(func(c *plane.Client) error {
				return runChronosStatesEnsure(cmd.Context(), c, project, cmd.OutOrStdout())
			})
		},
	}
	addProjectFlag(ensure, &project, "", "Plane project UUID (default: the configured project)")
	cmd.AddCommand(ensure)
	return cmd
}

func runChronosStatesEnsure(ctx context.Context, client *plane.Client, projectID string, out io.Writer) error {
	if projectID == "" {
		projectID = client.ProjectID()
	}
	created, err := client.EnsureLifecycleStates(ctx, projectID)
	if err != nil {
		return fmt.Errorf("ensure lifecycle states: %w", err)
	}
	if len(created) == 0 {
		fmt.Fprintf(out, "[✓] All lifecycle states already present for project %s (none created)\n", projectID)
	} else {
		fmt.Fprintf(out, "[✓] Created lifecycle states for project %s: %s\n", projectID, strings.Join(created, ", "))
	}
	return nil
}

func newChronosTicketCmd() *cobra.Command {
	cmd := newGroupCmd("ticket", "Read or set ticket status in DEV_STATUS.md and Plane", "")
	var file string
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List tickets from DEV_STATUS.md (falls back to Plane)",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChronosTicketList(cmd.Context(), file, cmd.OutOrStdout())
		},
	}
	set := &cobra.Command{
		Use:     "set <ticket> <status>",
		Short:   "Set a ticket's status in Plane and DEV_STATUS.md (backlog, todo, in progress, to verify, done)",
		Args:    exactArgs(2),
		Example: "dendrite chronos ticket set DEN-123 done\ndendrite chronos ticket set DEN-123 \"to verify\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := tracker.ParseStatus(args[1])
			if err != nil {
				return usageErrorf("%v. Accepted: backlog, todo, in progress, to verify, done", err)
			}
			return runChronosTicketSet(cmd.Context(), file, args[0], st, cmd.OutOrStdout())
		},
	}
	cmd.PersistentFlags().StringVarP(&file, "file", "f", devStatusFile, "Status file")
	cmd.AddCommand(list, set)
	return cmd
}

func runChronosTicketList(ctx context.Context, file string, out io.Writer) error {
	tr := tracker.NewMemoryTracker(file)
	if loadErr := tr.LoadFromMarkdown(file); loadErr != nil {
		client, err := newPlaneClient()
		if err == nil {
			if issues, err := client.ListPlaneIssues(ctx); err == nil && len(issues) > 0 {
				fmt.Fprintf(out, "[+] Registered Tickets from Plane (%d):\n", len(issues))
				for _, iss := range issues {
					fmt.Fprintf(out, "    [%s] %s\n", iss.State, iss.Name)
				}
				return nil
			}
		}
		return fmt.Errorf("load tickets from %s: %w", file, loadErr)
	}
	tickets, err := tr.ListTickets(ctx)
	if err != nil {
		return err
	}
	if globals.JSON {
		return writeJSON(out, tickets)
	}
	fmt.Fprintf(out, "[+] Registered Tickets (%d):\n", len(tickets))
	for _, t := range tickets {
		fmt.Fprintf(out, "    [%s] %s: %s (%s)\n", t.Status, t.ID, t.Title, t.Milestone)
	}
	return nil
}

func runChronosTicketSet(ctx context.Context, file, id string, st tracker.Status, out io.Writer) error {
	planeSynced := false
	if client, err := newPlaneClient(); err == nil && client != nil {
		if err := client.UpdateStatus(ctx, tracker.TicketID(id), st); err != nil {
			return fmt.Errorf("update ticket status in Plane: %w", err)
		}
		planeSynced = true
	}
	mdErr := devtool.UpdateTicketInMarkdown(ctx, file, id, st)
	if mdErr != nil && !planeSynced {
		return fmt.Errorf("update ticket: %w", mdErr)
	}
	msg := fmt.Sprintf("[✓] Successfully updated ticket %s to %s", id, st)
	if mdErr == nil {
		msg += " in " + file
	}
	if planeSynced {
		msg += " and reflected in Plane service"
	}
	fmt.Fprintln(out, msg)
	return nil
}

// runPlaneDocSync synchronizes a docs directory with Plane pages; `codex plane` calls it.
func runPlaneDocSync(ctx context.Context, direction, dir string, out io.Writer) error {
	var d plane.SyncDirection
	switch direction {
	case "push", "to-plane":
		d = plane.DirectionToPlane
	case "pull", "from-plane":
		d = plane.DirectionFromPlane
	case "sync", "both", "bidirectional":
		d = plane.DirectionBidirectional
	default:
		return usageErrorf("unknown sync direction %q: use push, pull or sync", direction)
	}
	return planeRun(func(client *plane.Client) error {
		fmt.Fprintf(out, "[+] Synchronizing project documentation (%s) with Plane (%s)...\n", d, dir)
		report, err := plane.SyncDocs(ctx, client, dir, d)
		if report != nil {
			fmt.Fprintf(out, "[✓] Documentation sync completed in %v\n", report.Duration)
			fmt.Fprintf(out, "    - Files Pushed to Plane:   %d\n", report.FilesPushed)
			fmt.Fprintf(out, "    - Files Pulled from Plane: %d\n", report.FilesPulled)
			fmt.Fprintf(out, "    - Pages Created:           %d\n", report.PagesCreated)
			fmt.Fprintf(out, "    - Pages Updated:           %d\n", report.PagesUpdated)
			if len(report.Errors) > 0 {
				fmt.Fprintf(out, "    - Warnings/Errors (%d):\n", len(report.Errors))
				for _, e := range report.Errors {
					fmt.Fprintf(out, "        * %s\n", e)
				}
			}
		}
		if err != nil {
			return fmt.Errorf("documentation sync: %w", err)
		}
		return nil
	})
}

func resolvePlaneTicketUUID(ctx context.Context, client *plane.Client, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if isPlaneUUID(raw) {
		return raw, nil
	}
	issues, err := client.ListPlaneIssues(ctx)
	if err != nil {
		return "", err
	}
	if seq, err := strconv.Atoi(raw); err == nil {
		for _, iss := range issues {
			if iss.SequenceID == seq {
				return iss.ID, nil
			}
		}
	}
	lower := strings.ToLower(raw)
	for _, iss := range issues {
		if strings.Contains(strings.ToLower(iss.Name), "["+lower+"]") {
			return iss.ID, nil
		}
	}
	for _, iss := range issues {
		if strings.Contains(strings.ToLower(iss.Name), lower) {
			return iss.ID, nil
		}
	}
	return "", fmt.Errorf("ticket '%s' not found in Plane", raw)
}
