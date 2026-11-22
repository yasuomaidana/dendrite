package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/rea"
)

// Parser reads the structure of a PE image; *coreclient.Client is the real one (the dendrite-core sidecar).
type Parser interface {
	PEParse(ctx context.Context, path string) (*coreclient.PEInfo, error)
}

// Indexer indexes the documents of an analysis and searches them; RagIndex is the real one (the RAG sidecar).
type Indexer interface {
	Index(ctx context.Context, dir string) error
	Search(ctx context.Context, dir, query string, k int) ([]Hit, error)
	Purge(ctx context.Context, dir string) error
}

// Hit is one passage of an analysis that matched a search.
type Hit struct {
	File    string  `json:"file"`
	Heading string  `json:"heading,omitempty"`
	Text    string  `json:"text"`
	Score   float32 `json:"score,omitempty"`
}

// Options say what to analyze with: an explicit id, the image format, the model tier (local or paid) and model.
type Options struct{ ID, Format, Tier, Model string }

// Result is an analysis after a run: its checkpoint and, once finished, its bounded evidence bundle.
type Result struct {
	State  State               `json:"state"`
	Bundle *rea.EvidenceBundle `json:"bundle,omitempty"`
}

var (
	formats = []string{"pe32", "pe64"}
	tiers   = []string{"local", "paid"}
)

const (
	defaultHits = 5
	maxHits     = 20
	maxHitChars = 1200
)

// Service runs analyses against a store, a parser and an index.
type Service struct {
	Store  *Store
	Parser Parser
	Index  Indexer
	Now    func() time.Time
}

// NewService is the real service: analyses kept under the Dendrite home, the shared dendrite-core client and the
// RAG sidecar.
func NewService() (*Service, error) {
	store, err := DefaultStore()
	if err != nil {
		return nil, err
	}
	return &Service{Store: store, Parser: coreclient.Default(), Index: NewRagIndex()}, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func oneOf(value string, allowed []string, what string) error {
	if !slices.Contains(allowed, value) {
		return fmt.Errorf("unknown %s %q: use %s", what, value, strings.Join(allowed, " or "))
	}
	return nil
}

func orDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// Analyze starts an analysis of the binary and runs it to the end, or to the first pause.
func (s *Service) Analyze(ctx context.Context, binary string, o Options) (*Result, error) {
	abs, _ := filepath.Abs(binary) // on failure abs is empty and the stat below reports the file unreadable
	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		return nil, fmt.Errorf("%s is not a readable file", binary)
	}
	format, tier := orDefault(o.Format, "pe64"), orDefault(o.Tier, "local")
	if err := errors.Join(oneOf(format, formats, "format"), oneOf(tier, tiers, "tier")); err != nil {
		return nil, err
	}
	now := s.now()
	st := State{ID: orDefault(o.ID, NewID(abs, now)), Binary: abs, Format: format, Tier: tier, Model: o.Model,
		Stage: StageNew, CreatedAt: now, UpdatedAt: now}
	if err := s.Store.Create(st); err != nil {
		return nil, err
	}
	return s.run(ctx, &st)
}

// Status reads the checkpoint of analysis id.
func (s *Service) Status(id string) (State, error) { return s.Store.Load(id) }

// Pause stops analysis id at the next stage boundary, whichever process is running it.
func (s *Service) Pause(id string) (State, error) {
	st, err := s.Store.Load(id)
	if err != nil {
		return State{}, err
	}
	if st.Stage == StageDone {
		return st, fmt.Errorf("analysis %s is already finished", id)
	}
	st.Paused, st.UpdatedAt = true, s.now()
	return st, s.Store.Save(&st)
}

// Resume continues analysis id from its last checkpoint, optionally on another tier or model.
func (s *Service) Resume(ctx context.Context, id string, o Options) (*Result, error) {
	st, err := s.Store.Load(id)
	if err != nil {
		return nil, err
	}
	if o.Tier != "" {
		if err := oneOf(o.Tier, tiers, "tier"); err != nil {
			return nil, err
		}
		st.Tier = o.Tier
	}
	st.Model = orDefault(o.Model, st.Model)
	st.Paused, st.UpdatedAt = false, s.now()
	if err := s.Store.Save(&st); err != nil {
		return nil, err
	}
	return s.run(ctx, &st)
}

// Purge deletes analysis id: its documents, its index and its checkpoint. It needs force.
func (s *Service) Purge(ctx context.Context, id string, force bool) error {
	if !force {
		return ErrNeedsForce
	}
	if _, err := s.Store.Load(id); err != nil {
		return err
	}
	purgeErr := s.Index.Purge(ctx, s.Store.WorkspaceDir(id))
	if err := s.Store.Delete(id); err != nil {
		return errors.Join(purgeErr, err)
	}
	if purgeErr != nil {
		return fmt.Errorf("analysis %s was deleted but its index was not purged: %w", id, purgeErr)
	}
	return nil
}

// Search returns the passages of a finished analysis that best match query, at most k (default 5) and never more
// than the 4,000-token evidence boundary, so an agent reads a few passages instead of the whole analysis.
func (s *Service) Search(ctx context.Context, id, query string, k int) ([]Hit, error) {
	st, err := s.Store.Load(id)
	if err != nil {
		return nil, err
	}
	if st.Stage != StageDone {
		return nil, ErrNotDone
	}
	if k <= 0 {
		k = defaultHits
	}
	hits, err := s.Index.Search(ctx, s.Store.WorkspaceDir(id), query, min(k, maxHits))
	if err != nil {
		return nil, err
	}
	return withinBudget(hits), nil
}

// withinBudget cuts long passages and drops what would pass the token boundary.
func withinBudget(hits []Hit) []Hit {
	var out []Hit
	tokens := 0
	for _, h := range hits {
		if len(h.Text) > maxHitChars {
			h.Text = h.Text[:maxHitChars] + "..."
		}
		tokens += rea.EstimateTokens(h.Text)
		if tokens > rea.MaxEvidenceTokens {
			break
		}
		out = append(out, h)
	}
	return out
}

// run takes the analysis through its remaining stages, saving a checkpoint after each. A pause requested by
// another process while a stage runs is read back at the checkpoint, so it is never overwritten and stops the
// analysis before the next stage.
func (s *Service) run(ctx context.Context, st *State) (*Result, error) {
	for st.Stage != StageDone {
		if st.Paused {
			return &Result{State: *st}, ErrPaused
		}
		if err := s.step(ctx, st); err != nil {
			st.Error = err.Error()
			_ = s.Store.Save(st)
			return &Result{State: *st}, err
		}
		if err := s.checkpoint(st); err != nil {
			return nil, err
		}
	}
	info, err := s.loadInfo(st.ID)
	if err != nil {
		return nil, err
	}
	return &Result{State: *st, Bundle: Bundle(*st, info)}, nil
}

// checkpoint saves the state after a stage, keeping a pause another process asked for in the meantime.
func (s *Service) checkpoint(st *State) error {
	fresh, err := s.Store.Load(st.ID)
	if err != nil {
		return err
	}
	st.Paused, st.Error, st.UpdatedAt = fresh.Paused, "", s.now()
	return s.Store.Save(st)
}

// step runs the stage the analysis is at and moves it to the next one.
func (s *Service) step(ctx context.Context, st *State) error {
	switch st.Stage {
	case StageNew:
		return s.parse(ctx, st)
	case StageParsed:
		return s.writeDocs(st)
	}
	return s.indexDocs(ctx, st)
}

// parse reads the PE structure and saves it, so later stages and a resume never parse again.
func (s *Service) parse(ctx context.Context, st *State) error {
	info, err := s.Parser.PEParse(ctx, st.Binary)
	if err != nil {
		return err
	}
	data, _ := json.MarshalIndent(info, "", "  ") // a struct of strings and numbers always marshals
	if err := fsutil.WriteFileAtomic(s.infoPath(st.ID), data); err != nil {
		return err
	}
	st.Stage = StageParsed
	return nil
}

// writeDocs writes the documents of the saved structure into the workspace.
func (s *Service) writeDocs(st *State) error {
	info, err := s.loadInfo(st.ID)
	if err != nil {
		return err
	}
	if err := writeWorkspace(s.Store.WorkspaceDir(st.ID), *st, info); err != nil {
		return err
	}
	st.Stage = StageWritten
	return nil
}

// indexDocs indexes the workspace so it can be searched.
func (s *Service) indexDocs(ctx context.Context, st *State) error {
	if err := s.Index.Index(ctx, s.Store.WorkspaceDir(st.ID)); err != nil {
		return err
	}
	st.Stage = StageDone
	return nil
}

func (s *Service) infoPath(id string) string { return filepath.Join(s.Store.Dir(id), "pe.json") }

func (s *Service) loadInfo(id string) (*coreclient.PEInfo, error) {
	data, err := os.ReadFile(s.infoPath(id))
	if err != nil {
		return nil, fmt.Errorf("analysis %s: the parsed structure is missing: %w", id, err)
	}
	var info coreclient.PEInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("analysis %s: unreadable parsed structure: %w", id, err)
	}
	return &info, nil
}
