package plane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"regexp"
	"strings"

	"Dendrite/pkg/tracker"
)

// ChecklistSource marks issues imported from a repository's markdown checklist.
const ChecklistSource = "dendrite-checklist"

// ChecklistItem is one top-level "- [ ]" line of a status document.
type ChecklistItem struct {
	Key       string
	Title     string
	Detail    string
	Milestone string
	Status    tracker.Status
}

// ChecklistImportReport summarises an ImportChecklist run.
type ChecklistImportReport struct {
	Created  int
	Existing int
	Modules  int
}

var (
	checklistLineRegex = regexp.MustCompile(`^- \[([ xX~/-])\]\s+(.+)$`)
	checklistHeadRegex = regexp.MustCompile(`^#{2,4}\s+(.+?)\s*$`)
	boldKeyRegex       = regexp.MustCompile(`^\*\*([A-Za-z][A-Za-z0-9]*-[0-9][0-9.]*)\*\*:?\s*(.*)$`)
	ticketKeyRegex     = regexp.MustCompile(`^(?i:ticket)\s+([0-9]+(?:\.[0-9]+)*):?\s*(.*)$`)
	plainKeyRegex      = regexp.MustCompile(`^([A-Z][A-Z0-9]*-[0-9][0-9.]*):?\s+(.*)$`)
)

// ParseChecklist extracts top-level checklist items from a status document.
// It understands "**ABC-1**: title", "Ticket 1.2.3: title" and "ABC-1 title — detail";
// lines without an ID are keyed by a hash of their title. Indented sub-bullets are ignored.
func ParseChecklist(md string) []ChecklistItem {
	var items []ChecklistItem
	milestone := ""
	for _, line := range strings.Split(md, "\n") {
		if m := checklistHeadRegex.FindStringSubmatch(line); m != nil {
			milestone = strings.TrimSpace(m[1])
			continue
		}
		m := checklistLineRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		item := ChecklistItem{Milestone: milestone, Status: checklistStatus(m[1])}
		rest := strings.TrimSpace(m[2])
		switch {
		case boldKeyRegex.MatchString(rest):
			s := boldKeyRegex.FindStringSubmatch(rest)
			item.Key, rest = s[1], s[2]
		case ticketKeyRegex.MatchString(rest):
			s := ticketKeyRegex.FindStringSubmatch(rest)
			item.Key, rest = s[1], s[2]
		case plainKeyRegex.MatchString(rest):
			s := plainKeyRegex.FindStringSubmatch(rest)
			item.Key, rest = s[1], s[2]
		}
		if title, detail, ok := strings.Cut(rest, " — "); ok {
			item.Title, item.Detail = strings.TrimSpace(title), strings.TrimSpace(detail)
		} else {
			item.Title = strings.TrimSpace(rest)
		}
		if item.Title == "" {
			continue
		}
		if item.Key == "" {
			sum := sha256.Sum256([]byte(milestone + "\x00" + item.Title))
			item.Key = "item-" + hex.EncodeToString(sum[:])[:10]
		}
		items = append(items, item)
	}
	return items
}

func checklistStatus(mark string) tracker.Status {
	switch mark {
	case "x", "X":
		return tracker.StatusDone
	case "~", "/", "-":
		return tracker.StatusInProgress
	default:
		return tracker.StatusTodo
	}
}

// ImportChecklist creates one Plane issue per item in the client's project, with one
// module per milestone. Items already imported (same external id) are left untouched.
func ImportChecklist(ctx context.Context, c *Client, items []ChecklistItem, sourceFile string) (*ChecklistImportReport, error) {
	report := &ChecklistImportReport{}

	states, err := c.ListStatesForProject(ctx, c.projectID)
	if err != nil {
		return nil, err
	}
	stateFor := make(map[tracker.Status]string)
	for _, s := range states {
		st := resolvePlaneState(s.Group, s.Name)
		if _, ok := stateFor[st]; !ok || strings.EqualFold(s.Name, formatStatusName(st)) {
			stateFor[st] = s.ID
		}
	}

	existing, err := c.ListPlaneIssues(ctx)
	if err != nil {
		return nil, err
	}
	imported := make(map[string]bool)
	for _, iss := range existing {
		if iss.ExternalSource == ChecklistSource {
			imported[iss.ExternalID] = true
		}
	}

	modules := make(map[string]string)
	if mods, err := c.ListModules(ctx); err == nil {
		for _, m := range mods {
			modules[m.Name] = m.ID
		}
	}

	for _, it := range items {
		if imported[it.Key] {
			report.Existing++
			continue
		}
		moduleID := ""
		if it.Milestone != "" {
			if id, ok := modules[it.Milestone]; ok {
				moduleID = id
			} else if mod, err := c.CreateModule(ctx, it.Milestone, "in-progress", ""); err == nil {
				modules[it.Milestone] = mod.ID
				moduleID = mod.ID
				report.Modules++
			}
		}
		desc := fmt.Sprintf("<p>Imported from <code>%s</code>", html.EscapeString(sourceFile))
		if it.Milestone != "" {
			desc += fmt.Sprintf(" (%s)", html.EscapeString(it.Milestone))
		}
		desc += ".</p>"
		if it.Detail != "" {
			desc += "<p>" + html.EscapeString(it.Detail) + "</p>"
		}
		if _, err := c.CreatePlaneIssueWithDetails(ctx, CreateIssueRequest{
			Name:            fmt.Sprintf("[%s] %s", it.Key, it.Title),
			DescriptionHTML: desc,
			StateID:         stateFor[it.Status],
			ModuleID:        moduleID,
			ExternalID:      it.Key,
			ExternalSource:  ChecklistSource,
		}); err != nil {
			return report, fmt.Errorf("import %s: %w", it.Key, err)
		}
		report.Created++
	}
	return report, nil
}
