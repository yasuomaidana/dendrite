package ticketlock

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"Dendrite/pkg/fsutil"
)

// PendingDecisionTTL is how long a decision made before any claim waits to be posted by the next claim.
const PendingDecisionTTL = 24 * time.Hour

// maxRawAnswer cuts an answer that could not be read as structured data.
const maxRawAnswer = 500

// Decision is one question put to the user (or settled by the agent) and the answer it got.
type Decision struct {
	Question string   `json:"question"`
	Answer   string   `json:"answer"`
	Options  []string `json:"options,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// DecisionsFromEvent reads the decisions out of an AskUserQuestion hook event: its questions with their
// options and the answers in the tool response, which is an object with an "answers" map (question to the
// chosen label) or a string holding one. An answer that cannot be read is kept as the raw response text
// rather than dropped.
func DecisionsFromEvent(in HookInput) []Decision {
	answers, raw := answersOf(in.ToolResponse)
	var out []Decision
	asked := map[string]bool{}
	for _, q := range in.ToolInput.Questions {
		d := Decision{Question: q.Question, Answer: answers[q.Question]}
		for _, o := range q.Options {
			d.Options = append(d.Options, o.Label)
		}
		if d.Answer == "" {
			d.Answer = firstNonEmpty(raw, "(no answer recorded)")
		}
		asked[q.Question] = true
		out = append(out, d)
	}
	var extra []string
	for q := range answers {
		if !asked[q] {
			extra = append(extra, q)
		}
	}
	sort.Strings(extra)
	for _, q := range extra {
		out = append(out, Decision{Question: q, Answer: answers[q]})
	}
	return out
}

// answersOf returns the answers map of a tool response, and the response as text when it has none.
func answersOf(resp json.RawMessage) (answers map[string]string, raw string) {
	var asString string
	if json.Unmarshal(resp, &asString) == nil {
		resp = json.RawMessage(asString)
	}
	var obj struct {
		Answers map[string]string `json:"answers"`
	}
	if json.Unmarshal(resp, &obj) == nil && len(obj.Answers) > 0 {
		return obj.Answers, ""
	}
	text := strings.TrimSpace(string(resp))
	if len(text) > maxRawAnswer {
		text = text[:maxRawAnswer] + "..."
	}
	return nil, text
}

// renderDecisions is the body of a decision comment: each question, its answer and the options it chose from.
func renderDecisions(ds []Decision, preface string) string {
	var parts []string
	if preface != "" {
		parts = append(parts, preface)
	}
	for _, d := range ds {
		block := "Q: " + d.Question + "\nA: " + d.Answer
		if len(d.Options) > 0 {
			block += "\nOptions: " + strings.Join(d.Options, " | ")
		}
		if d.Note != "" {
			block += "\nNote: " + d.Note
		}
		parts = append(parts, block)
	}
	return strings.Join(parts, "\n\n")
}

// RecordDecisions posts the decisions on the ticket as one decision comment.
func (l *Lifecycle) RecordDecisions(ctx context.Context, ticket string, ds []Decision, preface string) error {
	return l.Comment(ctx, ticket, "decision", renderDecisions(ds, preface))
}

// pendingDecisions are decisions made while no ticket was claimed, kept until the next claim.
type pendingDecisions struct {
	SavedAt   time.Time  `json:"saved_at"`
	Decisions []Decision `json:"decisions"`
}

func (s *Store) pendingPath(session string) string {
	return filepath.Join(s.Dir, "decisions", slug(session)+".json")
}

// AddPending keeps decisions of a session that has no claim.
func (s *Store) AddPending(session string, now time.Time, ds []Decision) error {
	pd, _ := readJSON[pendingDecisions](s.pendingPath(session))
	pd.SavedAt = now.UTC()
	pd.Decisions = append(pd.Decisions, ds...)
	data, _ := json.MarshalIndent(pd, "", "  ")
	if err := fsutil.WriteFileAtomic(s.pendingPath(session), append(data, '\n')); err != nil {
		return fmt.Errorf("keep decisions: %w", err)
	}
	return nil
}

// Pending returns the decisions kept since the given time, oldest first, and the files holding them.
func (s *Store) Pending(since time.Time) (ds []Decision, files []string) {
	entries, _ := os.ReadDir(filepath.Join(s.Dir, "decisions"))
	type kept struct {
		pd   pendingDecisions
		file string
	}
	var all []kept
	for _, e := range entries {
		path := filepath.Join(s.Dir, "decisions", e.Name())
		if pd, err := readJSON[pendingDecisions](path); err == nil && pd.SavedAt.After(since) {
			all = append(all, kept{pd, path})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].pd.SavedAt.Before(all[j].pd.SavedAt) })
	for _, k := range all {
		ds, files = append(ds, k.pd.Decisions...), append(files, k.file)
	}
	return ds, files
}

// adoptPendingDecisions posts the decisions made before this claim on the ticket. They are removed only once
// posted, so a failed post is retried by the next claim.
func (l *Lifecycle) adoptPendingDecisions(ctx context.Context, ticket string, now time.Time) {
	ds, files := l.Store.Pending(now.Add(-PendingDecisionTTL))
	if len(ds) == 0 {
		return
	}
	if err := l.RecordDecisions(ctx, ticket, ds, "Decisions made before this ticket was claimed:"); err != nil {
		return
	}
	for _, f := range files {
		_ = os.Remove(f)
	}
}

// SaveDecisions is the decision hook (after AskUserQuestion): the answers go to the ticket the working copy
// has claimed. With no claim they are kept locally and the next claim posts them. board is only opened when
// there is a ticket.
func SaveDecisions(ctx context.Context, l *Lifecycle, board func() (Board, error), in HookInput) (string, error) {
	ds := DecisionsFromEvent(in)
	if len(ds) == 0 {
		return "", nil
	}
	claim, claimed := l.Store.ClaimOfDir(in.Cwd)
	if !claimed {
		if err := l.Store.AddPending(firstNonEmpty(in.SessionID, "session"), l.Now(), ds); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d decision(s) kept locally: no ticket is claimed here, the next `dendrite chronos claim` posts them", len(ds)), nil
	}
	b, err := board()
	if err != nil {
		return "", fmt.Errorf("decisions not recorded on %s: %w", claim.Ticket, err)
	}
	l.Board = b
	if err := l.RecordDecisions(ctx, claim.Ticket, ds, ""); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d decision(s) recorded on %s", len(ds), claim.Ticket), nil
}

// CommentView is a ticket comment as `chronos comments` shows it.
type CommentView struct {
	ID      string `json:"id"`
	Kind    string `json:"kind,omitempty"`
	Created string `json:"created_at"`
	Text    string `json:"text"`
}

var (
	commentMarker = regexp.MustCompile(`^\[dendrite:([a-z]+)`)
	lineBreak     = regexp.MustCompile(`(?i)<br\s*/?>|</p>`)
)

// Comments lists the comments of a ticket, oldest first, optionally only those of one dendrite kind and only
// the last n (0 means all).
func (l *Lifecycle) Comments(ctx context.Context, ref, kind string, last int) ([]CommentView, error) {
	key := l.keyOf(ctx, ref)
	rows, err := l.Board.Svc.Comments(ctx, l.Board.Client, key)
	if err != nil {
		return nil, fmt.Errorf("read the comments of %s: %w", key, err)
	}
	var out []CommentView
	for _, c := range rows {
		text := strings.TrimSpace(html.UnescapeString(htmlTags.ReplaceAllString(lineBreak.ReplaceAllString(c.CommentHTML, "\n"), "")))
		v := CommentView{ID: c.ID, Created: c.CreatedAt, Text: text}
		if m := commentMarker.FindStringSubmatch(text); m != nil {
			v.Kind = m[1]
		}
		if kind == "" || v.Kind == kind {
			out = append(out, v)
		}
	}
	if last > 0 && len(out) > last {
		out = out[len(out)-last:]
	}
	return out, nil
}
