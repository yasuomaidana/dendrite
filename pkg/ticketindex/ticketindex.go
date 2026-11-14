// Package ticketindex makes Plane tickets searchable by Dendrite's retrieval. Each ticket is rendered
// to a Markdown file under $DENDRITE_HOME/tickets/<project>/; that folder is indexed as a project of
// its own, so `dendrite ask --corpus tickets` and the RAG tools answer questions about tickets
// (status, blockers, what a ticket is for) from ranked, cited text instead of a scan of the board.
package ticketindex

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// Corpus is the --corpus value that selects the ticket index.
const Corpus = "tickets"

// bulkRender is the number of rewritten tickets that counts as a first render; embedOnFirstUse is
// the time then spent embedding them before the query runs.
const (
	bulkRender      = 50
	embedOnFirstUse = 10 * time.Second
)

// FreshFor is how long a rendered index is reused before Plane is asked again.
const FreshFor = time.Minute

// Dir is the folder holding a project's rendered tickets.
func Dir(projectID string) (string, error) {
	home, err := projectid.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "tickets", projectID), nil
}

// Result describes one sync.
type Result struct {
	Dir     string
	Tickets int
	Written int
	Removed int
	// Reused is true when the index was fresh and Plane was not asked.
	Reused bool
}

const stampFile = ".synced"

// Sync renders the project's tickets into Dir, rewriting only files whose content changed and
// removing files of tickets that no longer exist. A sync newer than FreshFor is reused unless force.
func Sync(ctx context.Context, svc *planesvc.Service, client *plane.Client, force bool) (Result, error) {
	dir, err := Dir(client.ProjectID())
	if err != nil {
		return Result{}, err
	}
	res := Result{Dir: dir}
	if info, err := os.Stat(filepath.Join(dir, stampFile)); err == nil && !force && time.Since(info.ModTime()) < FreshFor {
		res.Reused = true
		return res, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, err
	}

	issues, err := client.ListPlaneIssues(ctx) // full records: descriptions included
	if err != nil {
		return res, fmt.Errorf("ticket index: list issues: %w", err)
	}
	snap, _, err := svc.Snapshot(ctx, client, false)
	if err != nil {
		return res, fmt.Errorf("ticket index: board snapshot: %w", err)
	}
	names := map[string]string{}
	for _, iss := range issues {
		names[iss.ID] = ticketLabel(iss)
	}

	want := map[string]bool{}
	for _, iss := range issues {
		if iss.ArchivedAt != nil {
			continue
		}
		file := fileName(iss)
		want[file] = true
		content := Render(iss, snap.Statuses[iss.ID], snap.Relations[iss.ID], names)
		path := filepath.Join(dir, file)
		if old, err := os.ReadFile(path); err == nil && string(old) == content {
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return res, err
		}
		res.Written++
	}
	res.Tickets = len(want)

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || want[e.Name()] {
			continue
		}
		if os.Remove(filepath.Join(dir, e.Name())) == nil {
			res.Removed++
		}
	}
	_ = os.WriteFile(filepath.Join(dir, stampFile), []byte(time.Now().Format(time.RFC3339)), 0o644)
	return res, nil
}

var bracketID = regexp.MustCompile(`^\s*\[([A-Za-z][A-Za-z0-9]*-\d+)\]\s*(.*)$`)

// ticketLabel is "DEN-12 title" for a titled ticket, else "#12 title".
func ticketLabel(iss plane.PlaneIssue) string {
	id, title := ticketID(iss)
	return id + " " + title
}

func ticketID(iss plane.PlaneIssue) (id, title string) {
	if m := bracketID.FindStringSubmatch(iss.Name); m != nil {
		return strings.ToUpper(m[1]), strings.TrimSpace(m[2])
	}
	return fmt.Sprintf("#%d", iss.SequenceID), strings.TrimSpace(iss.Name)
}

func fileName(iss plane.PlaneIssue) string {
	id, _ := ticketID(iss)
	return strings.NewReplacer("#", "plane-", "/", "-").Replace(id) + ".md"
}

// Render is the Markdown of one ticket.
func Render(iss plane.PlaneIssue, status tracker.Status, rel plane.IssueRelationsResponse, names map[string]string) string {
	id, title := ticketID(iss)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s %s\n\n", id, title)
	fmt.Fprintf(&b, "- Ticket: %s\n- Status: %s\n", id, tracker.StatusDisplayName(status))
	if iss.Priority != "" && iss.Priority != "none" {
		fmt.Fprintf(&b, "- Priority: %s\n", iss.Priority)
	}
	if iss.EstimatePoint != nil {
		fmt.Fprintf(&b, "- Estimate: %d points\n", *iss.EstimatePoint)
	}
	if iss.StartDate != nil || iss.TargetDate != nil {
		fmt.Fprintf(&b, "- Schedule: %s to %s\n", deref(iss.StartDate), deref(iss.TargetDate))
	}
	if !iss.UpdatedAt.IsZero() {
		fmt.Fprintf(&b, "- Updated: %s\n", iss.UpdatedAt.Format("2006-01-02"))
	}
	for _, g := range []struct {
		label string
		items []plane.IssueRelation
	}{{"Blocked by", rel.BlockedBy}, {"Blocks", rel.Blocking}, {"Relates to", rel.RelatesTo}, {"Duplicate of", rel.Duplicate}} {
		if line := relationLine(g.items, names); line != "" {
			fmt.Fprintf(&b, "- %s: %s\n", g.label, line)
		}
	}
	if text := Text(iss.DescriptionHTML); text != "" {
		fmt.Fprintf(&b, "\n## Description\n\n%s\n", text)
	}
	return b.String()
}

func relationLine(items []plane.IssueRelation, names map[string]string) string {
	var out []string
	for _, r := range items {
		if n := names[r.ID]; n != "" {
			out = append(out, n)
		} else if r.Name != "" {
			out = append(out, strings.TrimSpace(r.Name))
		}
	}
	sort.Strings(out)
	return strings.Join(out, "; ")
}

func deref(s *string) string {
	if s == nil || *s == "" {
		return "unscheduled"
	}
	return *s
}

var (
	reBreak = regexp.MustCompile(`(?i)<\s*(br\s*/?|/p|/div|/h[1-6]|/tr)\s*>`)
	reItem  = regexp.MustCompile(`(?i)<\s*li[^>]*>`)
	reTag   = regexp.MustCompile(`<[^>]*>`)
	reBlank = regexp.MustCompile(`\n{3,}`)
)

// Text turns Plane's description HTML into plain Markdown-ish text.
func Text(h string) string {
	if strings.TrimSpace(h) == "" {
		return ""
	}
	s := reBreak.ReplaceAllString(h, "\n")
	s = reItem.ReplaceAllString(s, "\n- ")
	s = reTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimSpace(reBlank.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

// Resolve maps the tickets corpus onto the ticket index: it syncs the folder and returns it as the
// project to search, with the corpus widened to everything in it. Any other corpus is returned
// unchanged with the given project path.
func Resolve(ctx context.Context, projectPath, corpus string) (path, resolvedCorpus string, err error) {
	if !strings.EqualFold(corpus, Corpus) {
		return projectPath, corpus, nil
	}
	client, err := plane.NewClientFromEnv()
	if err != nil {
		return "", "", fmt.Errorf("--corpus tickets needs Plane: %w", err)
	}
	res, err := Sync(ctx, planesvc.Default(), client, false)
	if err != nil {
		return "", "", err
	}
	if res.Written >= bulkRender {
		// A first render writes hundreds of files whose vectors do not exist yet; spend a bounded
		// moment embedding them so the first answer is not lexical-only. Later syncs touch a few files.
		_, _ = rag.NewRetriever().Index(ctx, res.Dir, rag.IndexOptions{EmbedBudgetMS: embedOnFirstUse.Milliseconds()})
	}
	return res.Dir, "all", nil
}
