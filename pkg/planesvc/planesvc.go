// Package planesvc is Dendrite's way to Plane: it asks the long-lived dendrite-plane sidecar
// (a pooled process behind a bounded queue). The sidecar is required: when it is missing or
// incompatible every call returns ErrSidecarMissing, which names the fix. There is no
// in-process copy of what the sidecar serves.
package planesvc

import (
	"context"
	"errors"
	"fmt"
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

// ErrSidecarMissing means the dendrite-plane sidecar could not be started (not installed,
// not executable, or speaking another protocol major).
var ErrSidecarMissing = errors.New("dendrite-plane sidecar not available: run `dendrite install` " +
	"(or `cargo build --release` in dendrite-plane-rs and copy target/release/dendrite-plane to $DENDRITE_HOME/bin)")

// call runs one sidecar method; a sidecar that cannot be started becomes ErrSidecarMissing with
// the cause attached, any other failure (a Plane error the sidecar reported) is returned as is.
func (s *Service) call(ctx context.Context, shared bool, method string, params, result interface{}) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("%w: no pool", ErrSidecarMissing)
	}
	var err error
	if shared {
		err = s.pool.CallShared(ctx, method, params, result)
	} else {
		err = s.pool.Call(ctx, method, params, result)
	}
	if errors.Is(err, sidecar.ErrUnavailable) {
		return fmt.Errorf("%w (%v)", ErrSidecarMissing, err)
	}
	return typed(err)
}

// typed gives the sidecar's error kinds the sentinel errors callers already test with errors.Is.
func typed(err error) error {
	var rpc *sidecar.RPCError
	if !errors.As(err, &rpc) {
		return err
	}
	switch rpc.Kind() {
	case "unauthorized":
		return fmt.Errorf("%w: %s", plane.ErrUnauthorized, rpc.Message)
	case "not_found":
		if strings.HasPrefix(rpc.Message, "no workflow state") {
			return fmt.Errorf("%w: %s", plane.ErrStateNotFound, rpc.Message)
		}
		return fmt.Errorf("%w: %s", plane.ErrIssueNotFound, rpc.Message)
	}
	return err
}

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

func target(c *plane.Client) map[string]any {
	return map[string]any{"base_url": c.BaseURL(), "workspace": c.WorkspaceSlug(), "project_id": c.ProjectID()}
}

// Snapshot returns the board of c's project. With fresh set, the sidecar's relation cache is
// bypassed.
func (s *Service) Snapshot(ctx context.Context, c *plane.Client, fresh bool) (*Snapshot, sidecar.Outcome, error) {
	params := target(c)
	params["fresh"] = fresh
	var snap Snapshot
	if err := s.call(ctx, true, "board.snapshot", params, &snap); err != nil {
		return nil, sidecar.Outcome{Origin: sidecar.OriginSidecar, Reason: err.Error()}, err
	}
	return &snap, sidecar.Outcome{Origin: sidecar.OriginSidecar}, nil
}

// SetStatus moves the ticket to state: the Zero-Mock gate runs first, then the sidecar's
// issue.set_status.
func (s *Service) SetStatus(ctx context.Context, c *plane.Client, ref tracker.TicketID, state tracker.Status) (sidecar.Outcome, error) {
	out := sidecar.Outcome{Origin: sidecar.OriginSidecar}
	if err := c.CheckZeroMockGate(ctx, ref, state); err != nil {
		return out, err
	}
	params := target(c)
	params["ref"] = string(ref)
	params["status"] = string(state)
	return out, s.call(ctx, false, "issue.set_status", params, nil)
}

// InvalidateRelations tells the sidecar that these issues' relations changed (empty: all of the
// project's). It is best effort: without a running sidecar there is nothing cached to drop.
func (s *Service) InvalidateRelations(ctx context.Context, c *plane.Client, issueIDs ...string) {
	if s == nil || s.pool == nil {
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
