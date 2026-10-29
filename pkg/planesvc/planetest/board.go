package planetest

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Workspace and project the Board answers for.
const (
	BoardWorkspace = "ws"
	BoardProject   = "proj"
)

// Board is a Plane that remembers: tickets with a workflow state, labels, assignees and comments, and the
// routes the dendrite-plane sidecar calls. Tests point the real sidecar at it (BaseURL, BoardWorkspace,
// BoardProject) and then read back what the sidecar left behind. Close it to simulate Plane being down.
type Board struct {
	*httptest.Server
	mu       sync.Mutex
	states   []boardState
	issues   []*boardIssue
	labels   map[string]string // id -> name
	comments map[string][]*BoardComment
	counter  int
	failures []*failure
}

type failure struct {
	match      string
	after, hit int
}

type boardState struct{ ID, Name, Group string }

type boardIssue struct {
	ID, Name, Description, State string
	Seq                          int
	Labels, Assignees            []string
}

// BoardComment is one comment of a ticket.
type BoardComment struct {
	ID, HTML, CreatedAt, UpdatedAt string
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// NewBoard starts a Board with the canonical workflow states and the tickets given as ref -> state name
// (for example "DEN-5": "In Progress"). A ticket's title is "[DEN-5] work" and its description "<p>goal</p>".
func NewBoard(t *testing.T, tickets map[string]string) *Board {
	t.Helper()
	b := &Board{
		states: []boardState{
			{"s-backlog", "Backlog", "backlog"}, {"s-todo", "To Do", "unstarted"}, {"s-prog", "In Progress", "started"},
			{"s-block", "Blocked", "started"}, {"s-verify", "To Verify", "started"}, {"s-done", "Done", "completed"},
			{"s-cancel", "Cancelled", "cancelled"},
		},
		labels:   map[string]string{},
		comments: map[string][]*BoardComment{},
	}
	refs := make([]string, 0, len(tickets))
	for ref := range tickets {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		var seq int
		_, _ = fmt.Sscanf(ref[strings.LastIndex(ref, "-")+1:], "%d", &seq)
		b.issues = append(b.issues, &boardIssue{
			ID: fmt.Sprintf("00000000-0000-0000-0000-%012d", seq), Name: "[" + ref + "] work", Description: "<p>goal</p>",
			Seq: seq, State: b.stateID(tickets[ref]),
		})
	}
	b.Server = httptest.NewServer(http.HandlerFunc(b.serve))
	t.Cleanup(b.Close)
	return b
}

// Fail makes the Board answer HTTP 500 to every request whose "METHOD path" contains match, after letting
// the first `after` such requests through. Use it to test what happens when Plane breaks half way.
func (b *Board) Fail(match string, after int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = append(b.failures, &failure{match: match, after: after})
}

// failing reports whether this request hits a failure rule; the caller holds the lock.
func (b *Board) failing(r *http.Request) bool {
	line := r.Method + " " + r.URL.Path
	for _, f := range b.failures {
		if strings.Contains(line, f.match) {
			f.hit++
			if f.hit > f.after {
				return true
			}
		}
	}
	return false
}

func (b *Board) stateID(name string) string {
	for _, s := range b.states {
		if s.Name == name {
			return s.ID
		}
	}
	return ""
}

func (b *Board) issue(ref string) *boardIssue {
	for _, i := range b.issues {
		if strings.HasPrefix(i.Name, "["+ref+"]") {
			return i
		}
	}
	return nil
}

// SetDescription replaces a ticket's description (HTML).
func (b *Board) SetDescription(ref, descriptionHTML string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.issue(ref).Description = descriptionHTML
}

// State is the name of a ticket's workflow state.
func (b *Board) State(ref string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.states {
		if s.ID == b.issue(ref).State {
			return s.Name
		}
	}
	return ""
}

// LabelNames are the names of a ticket's labels, in the order Plane holds them.
func (b *Board) LabelNames(ref string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []string{}
	for _, id := range b.issue(ref).Labels {
		out = append(out, b.labels[id])
	}
	return out
}

// Assignees are the user ids assigned to a ticket.
func (b *Board) Assignees(ref string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string{}, b.issue(ref).Assignees...)
}

// Comments are a ticket's comments, oldest first.
func (b *Board) Comments(ref string) []BoardComment {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []BoardComment{}
	for _, c := range b.comments[b.issue(ref).ID] {
		out = append(out, *c)
	}
	return out
}

// AddComment posts a comment as a person would; the HTML is stored as given.
func (b *Board) AddComment(ref, commentHTML string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.addComment(b.issue(ref).ID, commentHTML)
}

func (b *Board) addComment(issueID, commentHTML string) *BoardComment {
	b.counter++
	at := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC).Add(time.Duration(b.counter) * time.Second).Format(time.RFC3339)
	c := &BoardComment{ID: fmt.Sprintf("c-%d", b.counter), HTML: commentHTML, CreatedAt: at, UpdatedAt: at}
	b.comments[issueID] = append(b.comments[issueID], c)
	return c
}

// issueJSON is an issue as the self-hosted Plane sends it: the state is a bare id, with no state_detail
// (the sidecar resolves it through the states list), and the lists are [] and never null.
func (b *Board) issueJSON(i *boardIssue) map[string]any {
	return map[string]any{
		"id": i.ID, "name": i.Name, "sequence_id": i.Seq, "state": i.State,
		"description_html": i.Description, "description_stripped": strings.TrimSpace(htmlTag.ReplaceAllString(i.Description, " ")),
		"labels": append([]string{}, i.Labels...), "assignees": append([]string{}, i.Assignees...),
	}
}

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func ids(v any) []string {
	out := []string{}
	list, _ := v.([]any)
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (b *Board) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failing(r) {
		http.Error(w, "injected failure", http.StatusInternalServerError)
		return
	}

	// The self-hosted Plane serves the current user only here; /api/v1/users/me/ is a 404 (the sidecar falls back).
	if r.URL.Path == "/api/users/me/" {
		reply(w, map[string]string{"id": "u-1", "email": "agent@example.com", "display_name": "Agent"})
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	rest, ok := strings.CutPrefix(p, "workspaces/"+BoardWorkspace+"/projects/"+BoardProject+"/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch {
	case rest == "states/":
		rows := []map[string]string{}
		for _, s := range b.states {
			rows = append(rows, map[string]string{"id": s.ID, "name": s.Name, "group": s.Group})
		}
		reply(w, map[string]any{"results": rows})
	case rest == "labels/":
		b.labelRoute(w, r, body)
	case rest == "issues/":
		rows := []map[string]any{}
		for _, i := range b.issues {
			rows = append(rows, b.issueJSON(i))
		}
		reply(w, map[string]any{"results": rows, "next_page_results": false})
	case strings.HasPrefix(rest, "issues/"):
		b.issueRoute(w, r, strings.Split(strings.Trim(strings.TrimPrefix(rest, "issues/"), "/"), "/"), body)
	default:
		http.NotFound(w, r)
	}
}

func (b *Board) labelRoute(w http.ResponseWriter, r *http.Request, body map[string]any) {
	if r.Method == http.MethodPost {
		b.counter++
		id := fmt.Sprintf("aaaaaaaa-0000-0000-0000-%012d", b.counter)
		name, _ := body["name"].(string)
		b.labels[id] = name
		reply(w, map[string]string{"id": id, "name": name, "color": "#7C3AED"})
		return
	}
	rows := []map[string]string{}
	for id, name := range b.labels {
		rows = append(rows, map[string]string{"id": id, "name": name, "color": "#7C3AED"})
	}
	reply(w, map[string]any{"results": rows})
}

func (b *Board) issueRoute(w http.ResponseWriter, r *http.Request, parts []string, body map[string]any) {
	var issue *boardIssue
	for _, i := range b.issues {
		if i.ID == parts[0] {
			issue = i
		}
	}
	if issue == nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodPatch:
		if v, ok := body["state"].(string); ok {
			issue.State = v
		}
		if v, ok := body["name"].(string); ok {
			issue.Name = v
		}
		if v, ok := body["description_html"].(string); ok {
			issue.Description = v
		}
		if _, ok := body["labels"]; ok {
			issue.Labels = ids(body["labels"])
		}
		if _, ok := body["assignees"]; ok {
			issue.Assignees = ids(body["assignees"])
		}
		reply(w, b.issueJSON(issue))
	case len(parts) == 1:
		reply(w, b.issueJSON(issue))
	case parts[1] == "comments" && len(parts) == 2 && r.Method == http.MethodPost:
		html, _ := body["comment_html"].(string)
		reply(w, b.addComment(issue.ID, html))
	case parts[1] == "comments" && len(parts) == 2:
		reply(w, map[string]any{"results": b.commentRows(issue.ID)})
	case parts[1] == "comments" && len(parts) == 3 && r.Method == http.MethodPatch:
		for _, c := range b.comments[issue.ID] {
			if c.ID == parts[2] {
				c.HTML, _ = body["comment_html"].(string)
				reply(w, c)
				return
			}
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

// commentRows lists comments newest first, as Plane does, so clients must sort.
func (b *Board) commentRows(issueID string) []map[string]string {
	rows := []map[string]string{}
	list := b.comments[issueID]
	for i := len(list) - 1; i >= 0; i-- {
		c := list[i]
		rows = append(rows, map[string]string{"id": c.ID, "comment_html": c.HTML, "created_at": c.CreatedAt, "updated_at": c.UpdatedAt})
	}
	return rows
}

// Text is a comment's visible text: its HTML without tags and with entities decoded.
func (c BoardComment) Text() string {
	return strings.TrimSpace(html.UnescapeString(htmlTag.ReplaceAllString(c.HTML, " ")))
}
