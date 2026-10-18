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
	repo := opts.Repo
	if repo == nil {
		repo = vcs.NewRepoSync()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	idx := make([]int, len(s.Members))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return roleOrder[s.Members[idx[a]].Role] < roleOrder[s.Members[idx[b]].Role]
	})

	rep := &SyncReport{Synapse: s.ID}
	missing, drifted := false, false
	for _, i := range idx {
		m := &s.Members[i]
		ms := syncMember(ctx, repo, m, opts)
		missing = missing || ms.Missing
		if len(ms.Drift) > 0 || ms.Error != "" {
			drifted = true
		}
		if ms.LockedSHA != "" && ms.LockedSHA != m.LockedSHA {
			m.LockedSHA = ms.LockedSHA
			m.LockedAt = now().UTC().Truncate(time.Second)
			rep.Changed = true
		}
		rep.Members = append(rep.Members, ms)
	}

	if s.State != StateDraft && s.State != StateArchived {
		want := StateActive
		switch {
		case missing:
			want = StateBroken
		case drifted:
			want = StateDrifted
		}
		if want != s.State && s.Transition(want) == nil {
			rep.Changed = true
		}
	}
	rep.State = s.State

	if opts.Link {
		rep.Links = Link(s)
	}
	return rep
}

func syncMember(ctx context.Context, repo *vcs.RepoSync, m *Member, opts SyncOptions) MemberSync {
	ms := MemberSync{Alias: m.Alias, Role: m.Role, Source: m.Source, Remote: m.Remote, Ref: m.Ref, LockedSHA: m.LockedSHA}
	if ms.Source == "" {
		ms.Source = SourceLocal
	}

	if m.IsRemote() {
		if opts.NoFetch {
			return ms
		}
		sha, err := repo.ResolveRemoteRef(ctx, m.Remote, m.Ref)
		if err != nil {
			ms.Error = err.Error()
			return ms
		}
		ms.LockedSHA = sha
		return ms
	}

	path, err := Resolve(*m)
	if err != nil {
		ms.Missing = true
		ms.Error = fmt.Sprintf("local checkout does not resolve at %q (run 'dendrite synapse heal'): %v", m.PathHint, err)
		return ms
	}
	ms.Path = path

	ropts := vcs.RepoSyncOptions{Ref: m.Ref, NoFetch: opts.NoFetch, Rebase: opts.Rebase, Autostash: opts.Autostash, Merge: opts.Merge[m.Alias]}
	var st *vcs.RepoState
	if opts.Pull || ropts.Merge != "" {
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
	if st.Behind > 0 {
		ms.Drift = append(ms.Drift, fmt.Sprintf("%d commit(s) behind %s", st.Behind, st.Target))
	}
	if m.Ref != "" && st.Target != "" && !st.OnTarget && !st.Detached {
		ms.Drift = append(ms.Drift, fmt.Sprintf("checked out on %q but tracks %q", st.Branch, m.Ref))
	}
	if m.Ref != "" && st.Target == "" {
		ms.Drift = append(ms.Drift, fmt.Sprintf("tracked ref %q not found", m.Ref))
	}
	return ms
}
