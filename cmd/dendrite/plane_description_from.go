package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/dag"
	"Dendrite/pkg/tracker/plane"

	"github.com/spf13/cobra"
)

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
			if iss := plane.ResolveIssue(existing, b); iss != nil {
				blockerUUIDs = append(blockerUUIDs, iss.ID)
			} else {
				return fmt.Errorf("blocker %q not found in Plane; nothing was created", b)
			}
		}
	}

	created, _, err := planesvc.Default().CreateIssue(ctx, client, plane.CreateIssueRequest{
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
	created = prefixTicketTitle(ctx, client, created)
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
	target := plane.ResolveIssue(issues, ticket)
	if target == nil {
		return fmt.Errorf("ticket %s not found in Plane", ticket)
	}

	req := plane.UpdateIssueRequest{Name: name}
	if !plane.IsEmptyContext(desc) {
		req.DescriptionHTML = plane.MarkdownToHTML(desc)
	}
	updated, err := planesvc.Default().UpdateIssue(ctx, client, target.ID, req)
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
		if _, err := planesvc.Default().SetStatus(ctx, client, tracker.TicketID(id), st); err != nil {
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

// prefixTicketTitle gives a new ticket the "[ID-n]" title prefix the board and every resolver
// key on, using the project's identifier and the sequence number Plane assigned. A failure only
// leaves the plain title: the ticket still resolves by its sequence number.
func prefixTicketTitle(ctx context.Context, client *plane.Client, iss *plane.PlaneIssue) *plane.PlaneIssue {
	link, _, err := plane.LinkForDir(".")
	if err != nil || link == nil || link.Identifier == "" || iss == nil || iss.SequenceID == 0 {
		return iss
	}
	if strings.HasPrefix(strings.TrimSpace(iss.Name), "[") {
		return iss
	}
	name := fmt.Sprintf("[%s-%d] %s", link.Identifier, iss.SequenceID, iss.Name)
	updated, err := planesvc.Default().UpdateIssue(ctx, client, iss.ID, plane.UpdateIssueRequest{Name: name})
	if err != nil || updated == nil {
		return iss
	}
	return updated
}
