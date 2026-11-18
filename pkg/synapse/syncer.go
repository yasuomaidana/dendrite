package synapse

import (
	"context"
	"fmt"
	"sort"
	"time"

	"Dendrite/pkg/vcs"
)

// SyncOptions configures SyncSynapse.
type SyncOptions struct {
	// Pull updates local checkouts that are behind their tracked ref; without it Sync
	// only fetches and reports (dendrite synapse status).
	Pull bool
	// NoFetch skips fetching remotes; remote members keep their recorded lock.
	NoFetch bool
	// Rebase and Autostash are passed to vcs.RepoSync for local members.
	Rebase    bool
	Autostash bool
	// Enforce switches a clean local checkout that is on another branch to its tracked
	// branch (vcs.RepoSyncOptions.Enforce) instead of only reporting the drift.
	Enforce bool
	// Merge maps a member alias to a remote branch to merge into its checkout.
	Merge map[string]string
	// Link applies the synapse's sync strategy (go.work, cargo-patch) after syncing.
	Link bool
	// Repo performs the git operations (vcs.NewRepoSync() when nil).
	Repo *vcs.RepoSync
	// Now stamps lock times (time.Now when nil).
	Now func() time.Time
}

// MemberSync is the outcome of syncing one member.
type MemberSync struct {
	Alias     string         `json:"alias"`
	Role      Role           `json:"role"`
	Source    Source         `json:"source"`
	Path      string         `json:"path,omitempty"`
	Remote    string         `json:"remote,omitempty"`
	Ref       string         `json:"ref,omitempty"`
	LockedSHA string         `json:"locked_sha,omitempty"`
	Repo      *vcs.RepoState `json:"repo,omitempty"`
	// Missing marks a local member whose checkout does not resolve.
	Missing bool `json:"missing,omitempty"`
	// Drift lists why the member is not at its tracked ref; empty when in sync.
	Drift []string `json:"drift,omitempty"`
	Error string   `json:"error,omitempty"`
}

// SyncReport is the outcome of SyncSynapse for one synapse.
type SyncReport struct {
	Synapse string       `json:"synapse"`
	State   State        `json:"state"`
	Members []MemberSync `json:"members"`
	Links   []LinkResult `json:"links,omitempty"`
	// Changed reports whether s (locks or state) was modified and should be saved.
	Changed bool `json:"changed"`
}

// roleOrder syncs producers before peers before consumers so consumers link
// against freshly updated producers.
var roleOrder = map[Role]int{RoleProducer: 0, RolePeer: 1, RoleConsumer: 2}

// SyncSynapse brings every member of s to its tracked ref and records where each one
// landed. Local members are fetched and (with opts.Pull) fast-forwarded in place;
// remote members are resolved with ls-remote. Each member's commit is written to its
// lock, and the synapse moves to drifted when a member is behind or blocked, to broken
// when a local member's checkout is missing, and back to active once everything is in
// sync. With opts.Link the sync strategy is applied afterwards. s is modified in place;
// the caller saves it when the report says Changed.
func SyncSynapse(ctx context.Context, s *Synapse, opts SyncOptions) *SyncReport {
	repo, now := opts.defaults()
	rep := &SyncReport{Synapse: s.ID}
	missing, drifted := false, false
	for _, i := range syncOrder(s.Members) {
		m := &s.Members[i]
		ms := syncMember(ctx, repo, s, m, opts)
		missing = missing || ms.Missing
		drifted = drifted || len(ms.Drift) > 0 || ms.Error != ""
		rep.Changed = m.lock(ms.LockedSHA, now) || rep.Changed
		rep.Members = append(rep.Members, ms)
	}
	rep.Changed = s.settle(missing, drifted) || rep.Changed
	rep.State = s.State

	if opts.Link {
		rep.Links = Link(s)
	}
	return rep
}

// defaults fills in the git runner and the clock.
func (o SyncOptions) defaults() (*vcs.RepoSync, func() time.Time) {
	repo, now := o.Repo, o.Now
	if repo == nil {
		repo = vcs.NewRepoSync()
	}
	if now == nil {
		now = time.Now
	}
	return repo, now
}

// syncOrder is the member indexes in role order (roleOrder), stable within a role.
func syncOrder(members []Member) []int {
	idx := make([]int, len(members))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return roleOrder[members[idx[a]].Role] < roleOrder[members[idx[b]].Role]
	})
	return idx
}

// lock records sha as the member's commit and reports whether it differs from the last lock.
func (m *Member) lock(sha string, now func() time.Time) bool {
	if sha == "" || sha == m.LockedSHA {
		return false
	}
	m.LockedSHA = sha
	m.LockedAt = now().UTC().Truncate(time.Second)
	return true
}

// settle moves s to the state its members call for (broken when one is missing, drifted when
// one is behind or blocked, active otherwise) and reports whether it changed. Draft and
// archived synapses stay as they are.
func (s *Synapse) settle(missing, drifted bool) bool {
	if s.State == StateDraft || s.State == StateArchived {
		return false
	}
	want := StateActive
	switch {
	case missing:
		want = StateBroken
	case drifted:
		want = StateDrifted
	}
	return want != s.State && s.Transition(want) == nil
}

// RefreshStates re-derives the lifecycle state of every synapse in st from its members'
// checkouts as they are now, without fetching, and saves the manifests that changed. A state
// stored by an earlier status or sync therefore does not outlive its cause. Draft and
// archived synapses are left alone. It returns the synapses as they now stand.
func RefreshStates(ctx context.Context, st *Store) ([]*Synapse, error) {
	syns, err := st.List()
	if err != nil {
		return nil, err
	}
	for _, s := range syns {
		if s.State == StateDraft || s.State == StateArchived {
			continue
		}
		if rep := SyncSynapse(ctx, s, SyncOptions{NoFetch: true}); rep.Changed {
			if err := st.Save(s); err != nil {
				return syns, fmt.Errorf("saving synapse %s: %w", s.ID, err)
			}
		}
	}
	return syns, nil
}

// missingRefFix names the command that changes a tracked ref that does not exist: the
// member's own when it sets one, the synapse channel's otherwise.
func missingRefFix(s *Synapse, m *Member) string {
	if m.Ref != "" || m.Branch != "" {
		return fmt.Sprintf("the member sets it: dendrite synapse set-source %s %s local --ref <branch>", s.ID, m.Alias)
	}
	return fmt.Sprintf("the synapse channel sets it: dendrite synapse channel %s stable|dev", s.ID)
}

func syncMember(ctx context.Context, repo *vcs.RepoSync, s *Synapse, m *Member, opts SyncOptions) MemberSync {
	ref := m.TrackedRef(s.Channel)
	ms := MemberSync{Alias: m.Alias, Role: m.Role, Source: m.Source, Remote: m.Remote, Ref: ref, LockedSHA: m.LockedSHA}
	if ms.Source == "" {
		ms.Source = SourceLocal
	}
	if m.IsRemote() {
		return syncRemoteMember(ctx, repo, ms, opts.NoFetch)
	}

	path, err := Resolve(*m)
	if err != nil {
		ms.Missing = true
		ms.Error = fmt.Sprintf("local checkout does not resolve at %q (run 'dendrite synapse heal'): %v", m.PathHint, err)
		return ms
	}
	ms.Path = path

	ropts := vcs.RepoSyncOptions{Ref: ref, NoFetch: opts.NoFetch, Rebase: opts.Rebase, Autostash: opts.Autostash, Merge: opts.Merge[m.Alias], Enforce: opts.Enforce}
	var st *vcs.RepoState
	if opts.Pull || opts.Enforce || ropts.Merge != "" {
		st, err = repo.Sync(ctx, path, ropts)
	} else {
		st, err = repo.Inspect(ctx, path, ropts)
	}
	ms.Repo = st
	if err != nil {
		ms.Error = err.Error()
	}
	if st == nil {
		return ms
	}
	ms.LockedSHA = st.Head
	ms.Drift = memberDrift(s, m, ref, st)
	return ms
}

// syncRemoteMember resolves the commit a remote member's tracked ref points at; without a
// fetch it keeps the recorded lock.
func syncRemoteMember(ctx context.Context, repo *vcs.RepoSync, ms MemberSync, noFetch bool) MemberSync {
	if noFetch {
		return ms
	}
	sha, err := repo.ResolveRemoteRef(ctx, ms.Remote, ms.Ref)
	if err != nil {
		ms.Error = err.Error()
		return ms
	}
	ms.LockedSHA = sha
	return ms
}

// memberDrift lists why a local checkout is not at its tracked ref; empty when it is.
func memberDrift(s *Synapse, m *Member, ref string, st *vcs.RepoState) []string {
	var drift []string
	if st.Behind > 0 {
		drift = append(drift, fmt.Sprintf("%d commit(s) behind %s", st.Behind, st.Target))
	}
	if ref == "" {
		return drift
	}
	switch {
	case st.Target == "":
		drift = append(drift, fmt.Sprintf("tracked ref %q not found (%s)", ref, missingRefFix(s, m)))
	case !st.OnTarget && !st.Detached:
		drift = append(drift, fmt.Sprintf("checked out on %q but tracks %q", st.Branch, ref))
	}
	return drift
}
