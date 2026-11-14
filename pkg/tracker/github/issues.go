package github

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
)

// issueListLimit bounds `gh issue list`; it is far above the repository's issue count.
const issueListLimit = 5000

// ticketTitleRe matches the "[KEY-123]" prefix that ties an issue to its tracker ticket.
var ticketTitleRe = regexp.MustCompile(`^\s*\[([A-Za-z][A-Za-z0-9]*-[0-9]+)\]`)

// Issue is a GitHub issue that belongs to a tracker ticket.
type Issue struct {
	Number      int      `json:"number"`
	Title       string   `json:"title"`
	State       string   `json:"state"`       // OPEN or CLOSED
	StateReason string   `json:"stateReason"` // COMPLETED, NOT_PLANNED, REOPENED or ""
	Labels      []string `json:"labels"`
	TicketID    string   `json:"ticket_id"`
	// Comments are the issue's comments when the listing carried them (ListTicketIssues does);
	// CommentsKnown tells "no comments" from "not fetched".
	Comments      []Comment `json:"-"`
	CommentsKnown bool      `json:"-"`
}

// Open reports whether the issue is open.
func (i Issue) Open() bool { return i.State == "OPEN" }

// HasLabel reports whether the issue carries label.
func (i Issue) HasLabel(label string) bool {
	for _, l := range i.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// TicketIDFromTitle returns the ticket ID of an issue titled "[KEY-123] ...", or "".
func TicketIDFromTitle(title string) string {
	m := ticketTitleRe.FindStringSubmatch(title)
	if m == nil {
		return ""
	}
	return m[1]
}

// repoArgs scopes a gh command to the client's repository; with no repository
// configured gh resolves it from the current directory's git remote.
func (c *GitHubClient) repoArgs(args ...string) []string {
	if c.Repo != "" {
		args = append(args, "--repo", c.Repo)
	}
	return args
}

// ListTicketIssues returns every issue (open and closed) whose title starts with a
// ticket ID, keyed by that ID. When a ticket has several issues the open one with
// the lowest number wins, then the closed one with the highest number.
func (c *GitHubClient) ListTicketIssues(ctx context.Context) (map[string]Issue, error) {
	out, err := c.Executor.Execute(ctx, "gh", c.repoArgs("issue", "list",
		"--state", "all",
		"--limit", strconv.Itoa(issueListLimit),
		"--json", "number,title,state,stateReason,labels,comments")...)
	if err != nil {
		return nil, fmt.Errorf("github: list issues: %w", err)
	}
	var raw []struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		State       string `json:"state"`
		StateReason string `json:"stateReason"`
		Labels      []struct {
			Name string `json:"name"`
		} `json:"labels"`
		// A pointer tells "comments: []" from a listing that did not carry the field at all.
		Comments *[]rawComment `json:"comments"`
	}
	if out != "" {
		if err := json.Unmarshal([]byte(out), &raw); err != nil {
			return nil, fmt.Errorf("github: parse issue list: %w", err)
		}
	}
	issues := make(map[string]Issue)
	for _, r := range raw {
		id := TicketIDFromTitle(r.Title)
		if id == "" {
			continue
		}
		iss := Issue{Number: r.Number, Title: r.Title, State: r.State, StateReason: r.StateReason, TicketID: id}
		if r.Comments != nil {
			iss.Comments, iss.CommentsKnown = toComments(*r.Comments), true
		}
		for _, l := range r.Labels {
			iss.Labels = append(iss.Labels, l.Name)
		}
		if prev, ok := issues[id]; !ok || preferIssue(iss, prev) {
			issues[id] = iss
		}
	}
	return issues, nil
}

// preferIssue reports whether candidate should represent a ticket instead of current.
func preferIssue(candidate, current Issue) bool {
	if candidate.Open() != current.Open() {
		return candidate.Open()
	}
	if candidate.Open() {
		return candidate.Number < current.Number
	}
	return candidate.Number > current.Number
}

// findTicketIssue returns the issue number for ticketID, matching the title prefix exactly.
func (c *GitHubClient) findTicketIssue(ctx context.Context, ticketID string) (int, bool) {
	out, err := c.Executor.Execute(ctx, "gh", c.repoArgs("issue", "list",
		"--state", "all",
		"--search", fmt.Sprintf("%q in:title", "["+ticketID+"]"),
		"--json", "number,title",
		"--limit", "50")...)
	if err != nil || out == "" {
		return 0, false
	}
	var found []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if json.Unmarshal([]byte(out), &found) != nil {
		return 0, false
	}
	for _, f := range found {
		if TicketIDFromTitle(f.Title) == ticketID {
			return f.Number, true
		}
	}
	return 0, false
}

// Comment is a GitHub issue comment.
type Comment struct {
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// IssueComments returns the comments on issue number.
func (c *GitHubClient) IssueComments(ctx context.Context, number int) ([]Comment, error) {
	out, err := c.Executor.Execute(ctx, "gh", c.repoArgs("issue", "view", strconv.Itoa(number), "--json", "comments")...)
	if err != nil {
		return nil, fmt.Errorf("github: read comments of #%d: %w", number, err)
	}
	var raw struct {
		Comments []rawComment `json:"comments"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("github: parse comments of #%d: %w", number, err)
	}
	return toComments(raw.Comments), nil
}

// rawComment is a comment as `gh ... --json comments` prints it.
type rawComment struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

func toComments(raw []rawComment) []Comment {
	comments := make([]Comment, 0, len(raw))
	for _, rc := range raw {
		comments = append(comments, Comment{Author: rc.Author.Login, Body: rc.Body, CreatedAt: rc.CreatedAt})
	}
	return comments
}
