package memory

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/engram"
)

const (
	// week is the window of "new this week".
	week = 7 * 24 * time.Hour
	// switchMargin is how much better one store must score before the check prefers it alone.
	switchMargin = 0.1
	// recallWords is how many words of a memory's description are searched for.
	recallWords = 8
)

// Store is what the check reads from Engram.
type Store interface {
	// Healthy is nil when Engram's own diagnostics pass.
	Healthy(ctx context.Context) error
	Observations(ctx context.Context) ([]engram.Observation, error)
	// Search returns the titles of the top hits of a query.
	Search(ctx context.Context, query string) ([]string, error)
	// StartBytes is the size of the memory snapshot a session starts with.
	StartBytes(ctx context.Context) (int, error)
}

// Score is how one memory store did.
type Score struct {
	Memories   int     `json:"memories"`
	New7d      int     `json:"new_7d"`
	Recall     float64 `json:"recall"`
	Stale      float64 `json:"stale"`
	StartBytes int     `json:"start_bytes"`
	Score      float64 `json:"score"`
}

// Report is one weekly check: both stores' scores and the mode the scores point to.
type Report struct {
	Time          time.Time `json:"time"`
	Claude        Score     `json:"claude"`
	Engram        Score     `json:"engram"`
	EngramHealthy bool      `json:"engram_healthy"`
	Candidate     Mode      `json:"candidate"`
	Note          string    `json:"note,omitempty"`
}

// CheckInput says what to measure: Claude's memory folder, the repository whose paths memories may name, the
// time, and Engram (nil when it is not installed).
type CheckInput struct {
	ClaudeDir, Repo string
	Now             time.Time
	Store           Store
}

// Check measures both stores. Recall is how many of Claude's memories each store brings back: Claude by its
// MEMORY.md index (which is loaded every session), Engram by a search for the memory's description. Liveness is
// what was written in the last week (memories Engram only mirrored from Claude do not count), staleness the share
// of repository paths the memories name that no longer exist.
func Check(ctx context.Context, in CheckInput) (Report, error) {
	memories, err := ReadClaude(in.ClaudeDir)
	if err != nil {
		return Report{}, err
	}
	index, _ := os.ReadFile(filepath.Join(in.ClaudeDir, "MEMORY.md"))
	r := Report{Time: in.Now.UTC(), Claude: scoreClaude(memories, string(index), in)}
	r.Engram, r.EngramHealthy, r.Note = scoreEngram(ctx, in, memories)
	r.Candidate = candidate(r)
	return r, nil
}

func scoreClaude(memories []ClaudeMemory, index string, in CheckInput) Score {
	s := Score{Memories: len(memories), StartBytes: len(index)}
	var bodies []string
	listed := 0
	for _, m := range memories {
		bodies = append(bodies, m.Body)
		if strings.Contains(index, "("+m.File+")") {
			listed++
		}
		if in.Now.Sub(m.Modified) < week {
			s.New7d++
		}
	}
	s.Recall = ratio(listed, len(memories))
	s.Stale = stale(bodies, in.Repo)
	s.Score = weigh(s)
	return s
}

// scoreEngram returns Engram's score, whether it is usable, and why not when it is not.
func scoreEngram(ctx context.Context, in CheckInput, claude []ClaudeMemory) (Score, bool, string) {
	if in.Store == nil {
		return Score{}, false, "engram is not installed"
	}
	if err := in.Store.Healthy(ctx); err != nil {
		return Score{}, false, "engram is unhealthy: " + err.Error()
	}
	observations, err := in.Store.Observations(ctx)
	if err != nil {
		return Score{}, false, "engram cannot be read: " + err.Error()
	}
	s := Score{Memories: len(observations)}
	mirrored := map[string]bool{}
	for _, m := range claude {
		mirrored[m.Name] = true
	}
	var bodies []string
	for _, o := range observations {
		bodies = append(bodies, o.Content)
		if !mirrored[o.Title] && in.Now.Sub(o.Created()) < week {
			s.New7d++
		}
	}
	found := 0
	for _, m := range claude {
		if titles, err := in.Store.Search(ctx, recallQuery(m)); err == nil && contains(titles, m.Name) {
			found++
		}
	}
	s.Recall = ratio(found, len(claude))
	s.Stale = stale(bodies, in.Repo)
	s.StartBytes, _ = in.Store.StartBytes(ctx)
	s.Score = weigh(s)
	return s, true, ""
}

// recallQuery is the words of a memory's description (its name when it has none) that identify it.
func recallQuery(m ClaudeMemory) string {
	words := strings.Fields(m.Description)
	if len(words) == 0 {
		words = strings.Fields(strings.ReplaceAll(m.Name, "-", " "))
	}
	return strings.Join(words[:min(len(words), recallWords)], " ")
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func ratio(n, of int) float64 {
	if of == 0 {
		return 0
	}
	return float64(n) / float64(of)
}

// weigh is the score of a store: recall counts most, then being written to, then not going stale.
func weigh(s Score) float64 {
	return 0.6*s.Recall + 0.2*math.Min(float64(s.New7d), 3)/3 + 0.2*(1-s.Stale)
}

// pathRef finds repository paths in a memory.
var pathRef = regexp.MustCompile(`\b(?:pkg|cmd|docs|internal|skills|website)/[A-Za-z0-9_./-]*[A-Za-z0-9_]`)

// stale is the share of the repository paths named in texts that do not exist under repo.
func stale(texts []string, repo string) float64 {
	total, missing := 0, 0
	for _, text := range texts {
		for _, ref := range pathRef.FindAllString(text, -1) {
			total++
			if _, err := os.Stat(filepath.Join(repo, ref)); err != nil {
				missing++
			}
		}
	}
	return ratio(missing, total)
}

// candidate is the mode the scores point to: Claude alone when Engram is unusable or clearly worse, Engram
// alone when it is clearly better, both otherwise.
func candidate(r Report) Mode {
	if !r.EngramHealthy {
		return ClaudeOnly
	}
	switch diff := r.Engram.Score - r.Claude.Score; {
	case diff >= switchMargin:
		return EngramOnly
	case diff <= -switchMargin:
		return ClaudeOnly
	}
	return Both
}

// Summary is the one line that says how the stores compare.
func Summary(r Report) string {
	return fmt.Sprintf("engram %.2f vs claude %.2f (recall %.0f%% vs %.0f%%, new this week %d vs %d, stale %.0f%% vs %.0f%%)",
		r.Engram.Score, r.Claude.Score, r.Engram.Recall*100, r.Claude.Recall*100, r.Engram.New7d, r.Claude.New7d, r.Engram.Stale*100, r.Claude.Stale*100)
}

// EngramStore is the Store backed by the Engram binary, for the project of Dir.
type EngramStore struct{ Bin, Dir string }

// Healthy passes when `engram doctor` reports no blocked or failed check.
func (s EngramStore) Healthy(ctx context.Context) error {
	h, err := engram.Doctor(ctx, s.Bin)
	if err != nil {
		return err
	}
	if h.Blocked > 0 || h.Errors > 0 {
		return fmt.Errorf("doctor reports %d blocked and %d failed check(s)", h.Blocked, h.Errors)
	}
	return nil
}

// Observations lists the project's memories.
func (s EngramStore) Observations(ctx context.Context) ([]engram.Observation, error) {
	return engram.Export(ctx, s.Bin, s.Dir)
}

// Search returns the titles of the top five hits.
func (s EngramStore) Search(ctx context.Context, query string) ([]string, error) {
	return engram.Search(ctx, s.Bin, s.Dir, query, 5)
}

// StartBytes is the size of the snapshot a session starts with.
func (s EngramStore) StartBytes(ctx context.Context) (int, error) {
	text, err := engram.Snapshot(ctx, s.Bin, s.Dir, engram.SnapshotBytes)
	return len(text), err
}
