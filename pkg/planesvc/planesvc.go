// Package planesvc is Dendrite's fast path to Plane: it asks the long-lived dendrite-plane
// sidecar (a pooled process behind a bounded queue) and falls back to the in-process Go client
// whenever the sidecar is missing, incompatible, busy or fails, so Dendrite works with or
// without it installed. Both paths return the same Snapshot.
package planesvc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/tracker"
	"Dendrite/pkg/tracker/plane"
)

// Protocol is the dendrite-plane protocol major version this package speaks.
const Protocol = 1

// BinaryName is the sidecar executable (override its path with DENDRITE_DENDRITE_PLANE_BIN).
const BinaryName = "dendrite-plane"

// DisableEnv turns the sidecar path off when set to "off", "0" or "false" (tests, debugging).
const DisableEnv = "DENDRITE_PLANE_SIDECAR"

// Snapshot is the whole Plane board in one value: what a scheduler needs.
type Snapshot struct {
	Issues []plane.PlaneIssue `json:"issues"`
	// Statuses maps issue UUID to its lifecycle status.
	Statuses map[string]tracker.Status `json:"statuses"`
	// Relations maps issue UUID to its relations; only open (not Done/Cancelled) issues appear.
	Relations        map[string]plane.IssueRelationsResponse `json:"relations"`
	RelationErrors   int                                     `json:"relation_errors"`
	RelationsCached  int                                     `json:"relations_cached"`
	RelationsFetched int                                     `json:"relations_fetched"`
}

// Service answers Plane questions from the sidecar pool, with the Go client as fallback.
type Service struct {
	pool *sidecar.Pool
}

// New builds a service whose pool launches the located dendrite-plane binary.
func New(loc sidecar.Locator) *Service {
	return &Service{pool: sidecar.NewPool(sidecar.PoolConfig{
		Binary:   BinaryName,
		Major:    Protocol,
		Locator:  loc,
		ExtraEnv: cacheEnv,
	})}
}

// cacheEnv points the sidecar's relation cache at $DENDRITE_HOME/cache so the short-lived
// processes of separate `dendrite` invocations share it. An explicit DENDRITE_PLANE_CACHE_DIR wins.
func cacheEnv() []string {
	if os.Getenv("DENDRITE_PLANE_CACHE_DIR") != "" {
		return nil
	}
	home, err := projectid.HomeDir()
	if err != nil {
		return nil
	}
	return []string{"DENDRITE_PLANE_CACHE_DIR=" + filepath.Join(home, "cache", "plane")}
}

// NewWithPool is New with a caller-supplied pool (tests).
func NewWithPool(p *sidecar.Pool) *Service { return &Service{pool: p} }

var (
	defaultOnce sync.Once
	defaultSvc  *Service
)

// Default is the process-wide service. Its sidecar starts on first use and stops when idle.
func Default() *Service {
	defaultOnce.Do(func() {
		defaultSvc = New(sidecar.DefaultLocator())
		plane.SetRelationsChangedHook(func(c *plane.Client, issueIDs []string) {
			defaultSvc.InvalidateRelations(context.Background(), c, issueIDs...)
		})
	})
	return defaultSvc
}

func disabled() bool {
	switch strings.ToLower(os.Getenv(DisableEnv)) {
	case "off", "0", "false":
		return true
	}
	return false
}

func target(c *plane.Client) map[string]any {
	return map[string]any{"base_url": c.BaseURL(), "workspace": c.WorkspaceSlug(), "project_id": c.ProjectID()}
}

// Snapshot returns the board of c's project. With fresh set, the sidecar's relation cache is
// bypassed. The Outcome says which implementation answered.
func (s *Service) Snapshot(ctx context.Context, c *plane.Client, fresh bool) (*Snapshot, sidecar.Outcome, error) {
	reason := "sidecar disabled"
	if s != nil && s.pool != nil && !disabled() {
		params := target(c)
		params["fresh"] = fresh
		var snap Snapshot
		err := s.pool.CallShared(ctx, "board.snapshot", params, &snap)
		if err == nil {
			return &snap, sidecar.Outcome{Origin: sidecar.OriginSidecar}, nil
		}
		reason = err.Error()
	}
	snap, err := goSnapshot(ctx, c)
	return snap, sidecar.Outcome{Origin: sidecar.OriginFallback, Reason: reason}, err
}

// InvalidateRelations tells the sidecar that these issues' relations changed (empty: all of the
// project's). It is best effort: without a running sidecar there is nothing cached to drop.
func (s *Service) InvalidateRelations(ctx context.Context, c *plane.Client, issueIDs ...string) {
	if s == nil || s.pool == nil || disabled() {
		return
	}
	params := target(c)
	params["issue_ids"] = issueIDs
	_ = s.pool.Call(ctx, "cache.invalidate", params, nil)
}

// Close stops the sidecar.
func (s *Service) Close() error {
	if s == nil || s.pool == nil {
		return nil
	}
	return s.pool.Close()
}

// goSnapshot builds the same Snapshot with the in-process client: one deduplicated issue list,
// the project's states, and concurrent relation requests for the open issues only.
func goSnapshot(ctx context.Context, c *plane.Client) (*Snapshot, error) {
	issues, err := c.ListPlaneIssues(ctx)
	if err != nil {
		return nil, err
	}
	states, _ := c.ListStates(ctx)
	byState := make(map[string]tracker.Status, len(states))
	for _, st := range states {
		byState[st.ID] = plane.StatusFor(st.Group, st.Name)
	}
	snap := &Snapshot{
		Issues:    issues,
		Statuses:  make(map[string]tracker.Status, len(issues)),
		Relations: map[string]plane.IssueRelationsResponse{},
	}
	var open []string
	for _, iss := range issues {
		st, ok := byState[iss.State]
		if !ok {
			st = tracker.StatusBacklog
		}
		snap.Statuses[iss.ID] = st
		if st != tracker.StatusDone && st != tracker.StatusCancelled {
			open = append(open, iss.ID)
		}
	}
	rels, firstErr := c.GetRelationsFor(ctx, open)
	for id, r := range rels {
		if r != nil {
			snap.Relations[id] = *r
		}
	}
	if firstErr != nil {
		snap.RelationErrors = len(open) - len(snap.Relations)
	}
	snap.RelationsFetched = len(snap.Relations)
	return snap, nil
}
