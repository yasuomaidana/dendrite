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

// Options say what to analyze with: an explicit id, the format the target is expected to have (empty or auto: any),
// the model tier (local or paid) and model.
type Options struct{ ID, Format, Tier, Model string }

// Result is an analysis after a run: its checkpoint and, once finished, its bounded evidence bundle.
type Result struct {
	State  State               `json:"state"`
	Bundle *rea.EvidenceBundle `json:"bundle,omitempty"`
}

var tiers = []string{"local", "paid"}

const (
	defaultHits = 5
	maxHits     = 20
	maxHitChars = 1200
)

// Service runs analyses against a store, a parser and an index.
type Service struct {
	Store     *Store
	Inspector Inspector
	Functions FunctionLister // nil: the functions are not triaged
	Index     Indexer
	Now       func() time.Time
}

// NewService is the real service: analyses kept under the Dendrite home, the shared dendrite-core client and the
// RAG sidecar.
func NewService() (*Service, error) {
	store, err := DefaultStore()
	if err != nil {
		return nil, err
	}
	return &Service{Store: store, Inspector: &CoreInspector{Client: coreclient.Default()},
		Functions: coreclient.Default(), Index: NewRagIndex()}, nil
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

// Analyze starts an analysis of the target (a file of any format, or a directory) and runs it to the end, or to
// the first pause.
func (s *Service) Analyze(ctx context.Context, target string, o Options) (*Result, error) {
	abs, _ := filepath.Abs(target) // on failure abs is empty and the stat below reports the target unreadable
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("%s is not readable: %w", target, err)
	}
	format, err := normalizeFormat(o.Format)
	if err != nil {
		return nil, err
	}
	tier := orDefault(o.Tier, "local")
	if err := oneOf(tier, tiers, "tier"); err != nil {
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
	if st.Tier == TierPaid {
		st.Gate = "" // the paid tier is what the gate asks for
	}
	st.Model = orDefault(o.Model, st.Model)
	st.Paused, st.UpdatedAt = st.Gate != "", s.now() // a gate still standing keeps the analysis paused
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
	img, err := s.loadImage(st.ID)
	if err != nil {
		return nil, err
	}
	return &Result{State: *st, Bundle: Bundle(*st, img)}, nil
}

// checkpoint saves the state after a stage, keeping a pause another process asked for in the meantime.
func (s *Service) checkpoint(st *State) error {
	fresh, err := s.Store.Load(st.ID)
	if err != nil {
		return err
	}
	st.Paused, st.Error, st.UpdatedAt = fresh.Paused || st.Gate != "", "", s.now() // the pause gate holds until the paid tier is allowed
	return s.Store.Save(st)
}

// step runs the stage the analysis is at and moves it to the next one.
func (s *Service) step(ctx context.Context, st *State) error {
	switch st.Stage {
	case StageNew:
		return s.parse(ctx, st)
	case StageParsed:
		return s.writeDocs(st)
	case StageWritten:
		if s.Functions != nil {
			return s.triage(ctx, st)
		}
	}
	return s.indexDocs(ctx, st)
}

// parse inspects the target and saves what it found, so later stages and a resume never inspect again. A format
// the analysis was told to expect must be the format found.
func (s *Service) parse(ctx context.Context, st *State) error {
	img, err := s.Inspector.Inspect(ctx, st.Binary)
	if err != nil {
		return err
	}
	if st.Format != "" && st.Format != img.Format {
		return fmt.Errorf("expected a %s target but %s is %s", st.Format, filepath.Base(st.Binary), img.Format)
	}
	data, _ := json.MarshalIndent(img, "", "  ") // a struct of strings and numbers always marshals
	if err := fsutil.WriteFileAtomic(s.imagePath(st.ID), data); err != nil {
		return err
	}
	st.Format, st.Kind, st.Stage = img.Format, img.Kind, StageParsed
	return nil
}

// writeDocs writes the documents of the saved structure into the workspace.
func (s *Service) writeDocs(st *State) error {
	img, err := s.loadImage(st.ID)
	if err != nil {
		return err
	}
	if err := writeWorkspace(s.Store.WorkspaceDir(st.ID), *st, img); err != nil {
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

func (s *Service) imagePath(id string) string { return filepath.Join(s.Store.Dir(id), "image.json") }

// legacyPEPath is where analyses made before the format-neutral model kept the structure of a PE image.
func (s *Service) legacyPEPath(id string) string { return filepath.Join(s.Store.Dir(id), "pe.json") }

// loadImage reads what the inspection of analysis id found. An analysis saved before the format-neutral model
// holds a PE structure in pe.json, which is read as the image of a PE target.
func (s *Service) loadImage(id string) (*Image, error) {
	data, err := os.ReadFile(s.imagePath(id))
	if errors.Is(err, os.ErrNotExist) {
		return s.loadLegacyPE(id)
	}
	if err != nil {
		return nil, fmt.Errorf("analysis %s: the inspected structure is unreadable: %w", id, err)
	}
	var img Image
	if err := json.Unmarshal(data, &img); err != nil {
		return nil, fmt.Errorf("analysis %s: unreadable inspected structure: %w", id, err)
	}
	return &img, nil
}

func (s *Service) loadLegacyPE(id string) (*Image, error) {
	data, err := os.ReadFile(s.legacyPEPath(id))
	if err != nil {
		return nil, fmt.Errorf("analysis %s: the inspected structure is missing: %w", id, err)
	}
	var pe coreclient.PEInfo
	if err := json.Unmarshal(data, &pe); err != nil {
		return nil, fmt.Errorf("analysis %s: unreadable inspected structure: %w", id, err)
	}
	return coreclient.FromPE(&pe), nil
}
