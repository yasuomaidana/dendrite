package ticketlock

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/store/segmented"
)

// ErrNoPlan is returned when a ticket has no plan, in Plane or in the local store.
var ErrNoPlan = errors.New("no plan stored for this ticket")

// Plan is the markdown plan of a ticket, with `- [ ]` / `- [x]` steps.
type Plan struct {
	Markdown string `json:"markdown"`
	Version  int    `json:"version"`
	// PendingSteps counts the unchecked steps; NextStep is the first of them.
	PendingSteps int    `json:"pending_steps"`
	NextStep     string `json:"next_step,omitempty"`
	// Source says where the plan was read from: "plane" or "engram".
	Source string `json:"source,omitempty"`
}

var (
	planMarker = regexp.MustCompile(`\[dendrite:plan v=(\d+)\]`)
	planBody   = regexp.MustCompile(`(?s)<pre>(.*?)</pre>`)
	openStep   = regexp.MustCompile(`^\s*[-*]\s+\[ \]\s+(.*\S)\s*$`)
)

// newPlan summarises markdown as version v.
func newPlan(markdown string, v int) Plan {
	p := Plan{Markdown: markdown, Version: v}
	for _, line := range strings.Split(markdown, "\n") {
		if m := openStep.FindStringSubmatch(line); m != nil {
			p.PendingSteps++
			if p.NextStep == "" {
				p.NextStep = m[1]
			}
		}
	}
	return p
}

// planHTML is the stored form of a plan comment: a visible marker (Plane's editor may drop HTML comments)
// and the markdown, escaped, in a <pre> block.
func planHTML(p Plan) string {
	return fmt.Sprintf("<p>[dendrite:plan v=%d]</p><pre>%s</pre>", p.Version, html.EscapeString(p.Markdown))
}

// parsePlanHTML reads a plan back from the HTML of a comment; ok is false for any other comment.
func parsePlanHTML(commentHTML string) (Plan, bool) {
	head := planMarker.FindStringSubmatch(commentHTML)
	body := planBody.FindStringSubmatch(commentHTML)
	if head == nil || body == nil {
		return Plan{}, false
	}
	v, _ := strconv.Atoi(head[1])
	return newPlan(html.UnescapeString(body[1]), v), true
}

// PlanStore keeps the newest plan of every ticket on disk, so a plan survives Plane being down. Plans are
// content-addressed blobs (segmented.BlobStore); one small index file per ticket names the current one.
type PlanStore struct {
	Dir string
}

type planIndex struct {
	Version int    `json:"version"`
	Hash    string `json:"hash"`
}

func (s PlanStore) indexPath(ticket string) string {
	return filepath.Join(s.Dir, "index", slug(strings.ToUpper(ticket))+".json")
}

// Save stores p as the plan of ticket.
func (s PlanStore) Save(ticket string, p Plan) error {
	blobs := segmented.NewBlobStore(filepath.Join(s.Dir, "blobs"))
	hash := blobs.ComputeHash([]byte(p.Markdown))
	if _, _, err := blobs.WriteBlob(hash, []byte(p.Markdown)); err != nil {
		return fmt.Errorf("store plan: %w", err)
	}
	data, _ := json.Marshal(planIndex{Version: p.Version, Hash: hash})
	if err := fsutil.WriteFileAtomic(s.indexPath(ticket), data); err != nil {
		return fmt.Errorf("index plan: %w", err)
	}
	return nil
}

// Load reads the stored plan of ticket; ErrNoPlan when there is none.
func (s PlanStore) Load(ticket string) (Plan, error) {
	idx, err := readJSON[planIndex](s.indexPath(ticket))
	if errors.Is(err, os.ErrNotExist) {
		return Plan{}, ErrNoPlan
	}
	if err != nil {
		return Plan{}, err
	}
	data, err := segmented.NewBlobStore(filepath.Join(s.Dir, "blobs")).ReadBlob(idx.Hash)
	if err != nil {
		return Plan{}, fmt.Errorf("read plan: %w", err)
	}
	p := newPlan(string(data), idx.Version)
	p.Source = "engram"
	return p, nil
}
