// Package synapsesync propagates a project's state outward: it pushes unpushed git commits
// and refreshes the project's Plane board, then does the same for every peer the project is
// linked to through a synapse, and finally records each synapse's resulting health.
package synapsesync

import (
	"Dendrite/pkg/rag"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

// Options configures one sync pass.
type Options struct {
	// StartDir is the absolute path of the repository the sync is launched from; it is always synced first.
	StartDir string
	// SynapseID restricts the fan-out to one synapse; empty means every synapse StartDir belongs to.
	SynapseID string
	// All syncs every non-archived synapse in the store (daemon mode); StartDir is then optional.
	All       bool
	GitOnly   bool
	PlaneOnly bool
	// NoRAG skips the code-index refresh of each member.
	NoRAG  bool
	DryRun bool
	// Fetch refreshes remotes before computing ahead/behind.
	Fetch bool
	// ForcePlane pushes DEV_STATUS even when it is unchanged since the last sync.
	ForcePlane bool
	// Store defaults to synapse.DefaultStore().
	Store *synapse.Store
	// Plane is the base Plane client; nil disables the Plane half of the sync.
	Plane *plane.Client
	// DefaultPlaneProject binds a repository that is not in any synapse to a Plane project.
	DefaultPlaneProject string
	// StateDir holds per-project sync bookkeeping; defaults to $DENDRITE_HOME/sync.
	StateDir string
	// Indexer refreshes a repository's RAG index; the default asks the dendrite-rag sidecar for an
	// incremental index with a short embedding budget. A seam for tests.
	Indexer func(ctx context.Context, dir string) (RAGResult, error)
}

// RAGResult describes what the RAG half of the sync did for one member.
type RAGResult struct {
	Indexed      bool   `json:"indexed"`
	FilesChanged int    `json:"files_changed,omitempty"`
	EmbedPending int    `json:"embed_pending,omitempty"`
	DurationMS   int64  `json:"duration_ms,omitempty"`
	Skipped      string `json:"skipped,omitempty"`
}

// PlaneResult describes what the Plane half of the sync did for one member.
type PlaneResult struct {
	Project      string `json:"project"`
	StateChanges int    `json:"state_changes"`
	StatusPushed bool   `json:"status_pushed"`
	Tickets      int    `json:"tickets,omitempty"`
	Skipped      string `json:"skipped,omitempty"`
}

// MemberReport is the per-repository outcome.
type MemberReport struct {
	Synapse      string               `json:"synapse,omitempty"`
	Alias        string               `json:"alias"`
	ProjectID    string               `json:"project_id,omitempty"`
	Dir          string               `json:"dir"`
	PlaneProject string               `json:"plane_project,omitempty"`
	Status       synapse.MemberStatus `json:"status"`
	Repo         *devtool.RepoState   `json:"repo,omitempty"`
	Push         *devtool.PushOutcome `json:"push,omitempty"`
	Plane        *PlaneResult         `json:"plane,omitempty"`
	RAG          *RAGResult           `json:"rag,omitempty"`
	Errors       []string             `json:"errors,omitempty"`
}

// SynapseReport records a synapse's lifecycle state before and after the pass.
type SynapseReport struct {
	ID     string        `json:"id"`
	Before synapse.State `json:"before"`
	After  synapse.State `json:"after"`
}

// Report is the outcome of one sync pass.
type Report struct {
	DryRun   bool            `json:"dry_run"`
	Members  []MemberReport  `json:"members"`
	Synapses []SynapseReport `json:"synapses,omitempty"`
}

// Failed reports whether any member recorded an error.
func (r *Report) Failed() bool {
	for _, m := range r.Members {
		if len(m.Errors) > 0 {
			return true
		}
	}
	return false
}

// Target is one repository selected for syncing.
type Target struct {
	Synapse string
	Member  synapse.Member
}

// Discover returns the synapses in scope and the deduplicated member list, StartDir's repo first.
// A repository that belongs to no synapse yields a single standalone target.
func Discover(opts Options) ([]*synapse.Synapse, []Target, error) {
	st, err := storeOf(opts)
	if err != nil {
		return nil, nil, err
	}
	return discover(st, opts)
}

func discover(st *synapse.Store, opts Options) ([]*synapse.Synapse, []Target, error) {
	var err error
	startID := ""
	startDir := ""
	if opts.StartDir != "" {
		startDir = filepath.Clean(opts.StartDir)
		startID, _ = projectid.Read(startDir)
	}

	var syns []*synapse.Synapse
	switch {
	case opts.SynapseID != "":
		s, err := st.Load(opts.SynapseID)
		if err != nil {
			return nil, nil, err
		}
		syns = []*synapse.Synapse{s}
	case opts.All:
		all, err := st.List()
		if err != nil {
			return nil, nil, err
		}
		for _, s := range all {
			if s.State != synapse.StateArchived {
				syns = append(syns, s)
			}
		}
	case startID != "":
		if syns, err = st.ReferencingProject(startID); err != nil {
			return nil, nil, err
		}
	}

	var targets []Target
	seen := map[string]bool{}
	for _, s := range syns {
		for _, m := range s.Members {
			if seen[m.ProjectID] {
				continue
			}
			seen[m.ProjectID] = true
			t := Target{Synapse: s.ID, Member: m}
			if m.ProjectID == startID {
				targets = append([]Target{t}, targets...)
			} else {
				targets = append(targets, t)
			}
		}
	}

	if startDir != "" && (startID == "" || !seen[startID]) && opts.SynapseID == "" {
		standalone := Target{Member: synapse.Member{
			ProjectID:    startID,
			Alias:        filepath.Base(startDir),
			PathHint:     startDir,
			PlaneProject: opts.DefaultPlaneProject,
		}}
		targets = append([]Target{standalone}, targets...)
	}
	return syns, targets, nil
}

// Run performs one sync pass. Per-member failures are recorded in the report and never stop
// the remaining members; the returned error is reserved for discovery or bookkeeping failures.
func Run(ctx context.Context, opts Options) (*Report, error) {
	st, err := storeOf(opts)
	if err != nil {
		return nil, err
	}
	syns, targets, err := discover(st, opts)
	if err != nil {
		return nil, err
	}
	report := &Report{DryRun: opts.DryRun}
	for _, t := range targets {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		report.Members = append(report.Members, syncMember(ctx, opts, t))
	}

	for _, s := range syns {
		sr, changed := refreshState(s)
		report.Synapses = append(report.Synapses, sr)
		if changed && !opts.DryRun {
			if err := st.Save(s); err != nil {
				return report, fmt.Errorf("save synapse %s: %w", s.ID, err)
			}
		}
	}
	return report, nil
}

func storeOf(opts Options) (*synapse.Store, error) {
	if opts.Store != nil {
		return opts.Store, nil
	}
	return synapse.DefaultStore()
}

func syncMember(ctx context.Context, opts Options, t Target) MemberReport {
	m := t.Member
	mr := MemberReport{
		Synapse:      t.Synapse,
		Alias:        m.Alias,
		ProjectID:    m.ProjectID,
		Dir:          m.PathHint,
		PlaneProject: m.PlaneProject,
		Status:       synapse.StatusOK,
	}
	if mr.PlaneProject == "" && mr.Dir != "" {
		// Every editable repository names its Plane project in .dendrite/plane.json.
		if link, err := plane.ReadLink(mr.Dir); err == nil {
			mr.PlaneProject = link.Identifier
		}
	}
	if t.Synapse != "" {
		mr.Status = synapse.StatusOf(m)
		switch {
		case mr.Status == synapse.StatusRemote:
			// Consumed from its remote: there is no checkout here to push, index or reconcile.
			return mr
		case !mr.Status.Healthy():
			mr.Errors = append(mr.Errors, fmt.Sprintf("member %s; run `dendrite synapse heal`", mr.Status))
			return mr
		}
		// Work on the repository where it is now, which may differ from the manifest's hint.
		if dir, err := synapse.Resolve(m); err == nil {
			mr.Dir = dir
		}
	}

	if !opts.PlaneOnly {
		syncGit(ctx, opts, &mr)
	}
	if !opts.GitOnly {
		syncPlane(ctx, opts, &mr)
	}
	if !opts.GitOnly && !opts.PlaneOnly && !opts.NoRAG {
		syncRAG(ctx, opts, &mr)
	}
	return mr
}

// syncRAG brings the member's code index up to date so every project of a synapse answers
// retrieval questions about current code. It is skipped while HEAD is the commit last indexed and
// the working tree is clean: an unchanged repository costs one git call.
func syncRAG(ctx context.Context, opts Options, mr *MemberReport) {
	res := &RAGResult{}
	mr.RAG = res
	head, err := gitHead(ctx, mr.Dir)
	if err != nil {
		res.Skipped = "not a git repository"
		return
	}
	clean := mr.Repo != nil && len(mr.Repo.Dirty) == 0
	key := stateKey(mr)
	prev, _ := loadSyncState(opts, key)
	if clean && prev.RAGHead == head {
		res.Skipped = "unchanged since the last index"
		return
	}
	if opts.DryRun {
		res.Skipped = "dry-run: would refresh the index"
		return
	}
	indexer := opts.Indexer
	if indexer == nil {
		indexer = defaultIndexer
	}
	out, err := indexer(ctx, mr.Dir)
	if err != nil {
		res.Skipped = firstLine(err.Error())
		mr.Errors = append(mr.Errors, "rag index: "+err.Error())
		return
	}
	*res = out
	res.Indexed = true
	// Remember the commit only for a clean tree; a dirty one is re-indexed next time.
	prev.RAGHead = ""
	if clean {
		prev.RAGHead = head
	}
	prev.RAGAt = time.Now().UTC()
	if err := saveSyncState(opts, key, prev); err != nil {
		mr.Errors = append(mr.Errors, err.Error())
	}
}

func gitHead(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// ragEmbedBudget bounds the embedding done per member in one sync pass; vectors for the rest follow
// on later passes and queries.
const ragEmbedBudget = 3 * time.Second

// defaultIndexer is a variable so the package tests do not need a dendrite-rag binary.
var defaultIndexer = func(ctx context.Context, dir string) (RAGResult, error) {
	rep, err := rag.NewRetriever().Index(ctx, dir, rag.IndexOptions{EmbedBudgetMS: ragEmbedBudget.Milliseconds()})
	if err != nil {
		return RAGResult{}, err
	}
	return RAGResult{FilesChanged: rep.FilesChanged, EmbedPending: rep.EmbedPending, DurationMS: rep.DurationMS}, nil
}

func syncGit(ctx context.Context, opts Options, mr *MemberReport) {
	repo, err := devtool.InspectRepo(ctx, mr.Dir, opts.Fetch)
	if err != nil {
		mr.Errors = append(mr.Errors, err.Error())
		return
	}
	mr.Repo = repo
	if repo.Ahead == 0 {
		return
	}
	switch {
	case repo.Behind > 0:
		mr.Push = &devtool.PushOutcome{Skipped: true, Branch: repo.Branch,
			Reason: fmt.Sprintf("diverged (%d ahead, %d behind); pull --rebase first", repo.Ahead, repo.Behind)}
	case opts.DryRun:
		mr.Push = &devtool.PushOutcome{Skipped: true, Branch: repo.Branch,
			Reason: fmt.Sprintf("dry-run: would push %d commit(s)", repo.Ahead)}
	default:
		out := devtool.PushCurrentBranch(ctx, mr.Dir)
		mr.Push = &out
		if out.Err != nil {
			mr.Errors = append(mr.Errors, out.Err.Error())
		}
	}
}

func syncPlane(ctx context.Context, opts Options, mr *MemberReport) {
	res := &PlaneResult{Project: mr.PlaneProject}
	mr.Plane = res
	if mr.PlaneProject == "" {
		res.Skipped = "no Plane project bound (dendrite synapse set-plane)"
		return
	}
	if opts.Plane == nil {
		res.Skipped = "Plane client unavailable"
		return
	}

	client, err := plane.ClientForProject(ctx, opts.Plane, mr.PlaneProject)
	if err != nil {
		mr.Errors = append(mr.Errors, err.Error())
		return
	}
	rec, err := client.ReconcileStates(ctx, client.ProjectID(), opts.DryRun)
	if err != nil {
		mr.Errors = append(mr.Errors, err.Error())
		return
	}
	res.StateChanges = len(rec.Changes)

	content, err := os.ReadFile(filepath.Join(mr.Dir, "DEV_STATUS.md"))
	if err != nil {
		res.Skipped = "no DEV_STATUS.md"
		return
	}
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	key := stateKey(mr)
	prev, _ := loadSyncState(opts, key)
	if !opts.ForcePlane && prev.DevStatusSHA256 == hash {
		res.Skipped = "DEV_STATUS unchanged since last sync"
		return
	}
	if opts.DryRun {
		res.Skipped = "dry-run: would push DEV_STATUS"
		return
	}
	rep, err := plane.PushStatusToPlaneIn(ctx, client, string(content), mr.Dir)
	if err != nil {
		mr.Errors = append(mr.Errors, err.Error())
		return
	}
	res.StatusPushed = true
	res.Tickets = rep.TotalTickets
	prev.DevStatusSHA256, prev.SyncedAt = hash, time.Now().UTC()
	if err := saveSyncState(opts, key, prev); err != nil {
		mr.Errors = append(mr.Errors, err.Error())
	}
}

// refreshState moves a synapse to broken when a member is unresolvable and back to active once
// every member resolves again. Drift (contract divergence) is not evaluated here.
func refreshState(s *synapse.Synapse) (SynapseReport, bool) {
	sr := SynapseReport{ID: s.ID, Before: s.State, After: s.State}
	healthy := true
	for _, status := range s.Health() {
		if !status.Healthy() {
			healthy = false
			break
		}
	}
	target := s.State
	switch {
	case !healthy && (s.State == synapse.StateActive || s.State == synapse.StateDrifted):
		target = synapse.StateBroken
	case healthy && s.State == synapse.StateBroken:
		target = synapse.StateActive
	}
	if target == s.State || s.Transition(target) != nil {
		return sr, false
	}
	sr.After = s.State
	return sr, true
}

type syncState struct {
	DevStatusSHA256 string    `json:"dev_status_sha256"`
	SyncedAt        time.Time `json:"synced_at"`
	// RAGHead is the commit whose clean tree was last indexed; RAGAt when.
	RAGHead string    `json:"rag_head,omitempty"`
	RAGAt   time.Time `json:"rag_at,omitempty"`
}

func stateKey(mr *MemberReport) string {
	if mr.ProjectID != "" {
		return mr.ProjectID + "-" + mr.PlaneProject
	}
	sum := sha256.Sum256([]byte(mr.Dir))
	return "path_" + hex.EncodeToString(sum[:8]) + "-" + mr.PlaneProject
}

func stateDir(opts Options) (string, error) {
	if opts.StateDir != "" {
		return opts.StateDir, nil
	}
	home, err := projectid.HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "sync"), nil
}

func loadSyncState(opts Options, key string) (syncState, error) {
	var s syncState
	dir, err := stateDir(opts)
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(filepath.Join(dir, key+".json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

func saveSyncState(opts Options, key string, s syncState) error {
	dir, err := stateDir(opts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(s, "", "  ") // syncState always marshals
	return os.WriteFile(filepath.Join(dir, key+".json"), data, 0o644)
}

// DefaultInterval is the watch period used when none is configured.
const DefaultInterval = 10 * time.Minute

// IntervalFromEnv reads DENDRITE_SYNC_INTERVAL (a Go duration such as "5m"); an empty or
// invalid value yields DefaultInterval and "0" disables periodic syncing (returns 0).
func IntervalFromEnv() time.Duration {
	raw := os.Getenv("DENDRITE_SYNC_INTERVAL")
	if raw == "" {
		return DefaultInterval
	}
	if raw == "0" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return DefaultInterval
	}
	return d
}

// Watch runs a sync pass immediately and then every interval until ctx is cancelled, handing
// each outcome to onPass. It is the hook long-running daemons use to keep synapses current.
func Watch(ctx context.Context, opts Options, interval time.Duration, onPass func(*Report, error)) error {
	if interval <= 0 {
		return fmt.Errorf("sync watch interval must be positive, got %s", interval)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		onPass(Run(ctx, opts))
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
