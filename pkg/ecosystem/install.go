// Package ecosystem installs the Dendrite ecosystem from git: every member repository is
// cloned next to the others and kept on the branch its synapse channel names (stable:
// main, dev: develop), unless the member overrides it with its own ref.
package ecosystem

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/vcs"
)

//go:embed dendrite-ecosystem.toml
var definitionTOML []byte

// ControlAlias is the member holding Dendrite itself.
const ControlAlias = "control"

// Definition returns the ecosystem shipped with this build. Member path hints are
// directory names relative to the install root.
func Definition() (*synapse.Synapse, error) {
	return synapse.Unmarshal(definitionTOML)
}

// Options configures Install.
type Options struct {
	// Root is the directory members are cloned into (<root>/<path_hint>).
	Root string
	// Channel selects the branch members track (stable: main, dev: develop).
	Channel synapse.Channel
	// Refs overrides the tracked ref per member alias; an empty value clears an override.
	Refs map[string]string
	// Dirs places members at explicit directories (e.g. the running control checkout),
	// taking precedence over the stored manifest and Root.
	Dirs map[string]string
	// Definition is the ecosystem to install; Definition() when nil.
	Definition *synapse.Synapse
	// Store persists the resulting manifest; synapse.DefaultStore() when nil.
	Store *synapse.Store
	// Repo runs the git operations; vcs.NewRepoSync() when nil.
	Repo *vcs.RepoSync
}

// MemberResult reports what Install did with one member.
type MemberResult struct {
	Alias  string `json:"alias"`
	Dir    string `json:"dir"`
	Ref    string `json:"ref"`
	Cloned bool   `json:"cloned,omitempty"`
}

// Result is the outcome of Install.
type Result struct {
	Synapse *synapse.Synapse    `json:"synapse"`
	Members []MemberResult      `json:"members"`
	Sync    *synapse.SyncReport `json:"sync"`
}

// Install clones every missing member of the ecosystem at its tracked ref, records the
// manifest (channel, overrides, absolute checkout paths) in the synapse store, then syncs
// every member with branch enforcement: a clean checkout on another branch is switched to
// the tracked one and fast-forwarded. An existing manifest keeps its member locations and
// overrides. It returns an error when any member could not be put on its tracked ref.
func Install(ctx context.Context, opts Options) (*Result, error) {
	if opts.Channel.DefaultRef() == "" {
		return nil, fmt.Errorf("invalid channel %q (want stable or dev)", opts.Channel)
	}
	def := opts.Definition
	if def == nil {
		var err error
		if def, err = Definition(); err != nil {
			return nil, fmt.Errorf("embedded ecosystem definition: %w", err)
		}
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, err
	}
	store := opts.Store
	if store == nil {
		if store, err = synapse.DefaultStore(); err != nil {
			return nil, err
		}
	}
	repo := opts.Repo
	if repo == nil {
		repo = vcs.NewRepoSync()
	}
	for alias := range opts.Refs {
		if _, ok := def.Member(alias); !ok {
			return nil, fmt.Errorf("--ref: the ecosystem has no member %q", alias)
		}
	}

	s := merge(def, existing(store, def.ID))
	s.Channel = opts.Channel
	if s.State == "" || s.State == synapse.StateDraft {
		s.State = synapse.StateActive
	}

	res := &Result{Synapse: s}
	for i := range s.Members {
		m := &s.Members[i]
		if ref, ok := opts.Refs[m.Alias]; ok && ref != m.Ref {
			m.Ref = ref
			m.LockedSHA = ""
		}
		dir := m.PathHint
		if d, ok := opts.Dirs[m.Alias]; ok {
			dir = d
		} else if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		ref := m.TrackedRef(s.Channel)
		mr := MemberResult{Alias: m.Alias, Dir: dir, Ref: ref}
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			if m.Remote == "" {
				return nil, fmt.Errorf("member %q is missing at %s and has no remote to clone", m.Alias, dir)
			}
			if err := clone(ctx, repo, m.Remote, ref, dir); err != nil {
				return nil, fmt.Errorf("cloning %s: %w", m.Alias, err)
			}
			mr.Cloned = true
		}
		if err := ensureProjectID(ctx, repo, dir, m.ProjectID); err != nil {
			return nil, fmt.Errorf("member %q: %w", m.Alias, err)
		}
		m.PathHint = dir
		res.Members = append(res.Members, mr)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}

	rep := synapse.SyncSynapse(ctx, s, synapse.SyncOptions{Pull: true, Enforce: true, Link: true, Repo: repo})
	res.Sync = rep
	if err := store.Save(s); err != nil {
		return res, fmt.Errorf("saving synapse %s: %w", s.ID, err)
	}
	var failed []string
	for _, m := range rep.Members {
		if m.Error != "" {
			failed = append(failed, fmt.Sprintf("%s: %s", m.Alias, m.Error))
		}
	}
	if len(failed) > 0 {
		return res, fmt.Errorf("members not on their tracked ref:\n  %s", strings.Join(failed, "\n  "))
	}
	return res, nil
}

// existing loads a stored manifest with id, or nil.
func existing(store *synapse.Store, id string) *synapse.Synapse {
	s, err := store.Load(id)
	if err != nil {
		return nil
	}
	return s
}

// merge overlays what the user already configured (member locations, overrides, Plane
// bindings, extra members, lifecycle state) on the shipped definition.
func merge(def, prev *synapse.Synapse) *synapse.Synapse {
	s := *def
	s.Members = append([]synapse.Member(nil), def.Members...)
	if prev == nil {
		return &s
	}
	s.State, s.CreatedAt, s.Contracts = prev.State, prev.CreatedAt, prev.Contracts
	if s.Sync.Strategy == "" {
		s.Sync = prev.Sync
	}
	for k, v := range prev.Parity.Commands {
		if _, ok := s.Parity.Commands[k]; !ok {
			if s.Parity.Commands == nil {
				s.Parity.Commands = map[string]string{}
			}
			s.Parity.Commands[k] = v
		}
	}
	for _, pm := range prev.Members {
		m, ok := s.Member(pm.Alias)
		if !ok {
			s.Members = append(s.Members, pm)
			continue
		}
		if pm.PathHint != "" {
			m.PathHint = pm.PathHint
		}
		if m.Remote == "" {
			m.Remote = pm.Remote
		}
		m.Ref, m.Source, m.Module = pm.Ref, pm.Source, pm.Module
		m.LockedSHA, m.LockedAt = pm.LockedSHA, pm.LockedAt
		if pm.PlaneProject != "" {
			m.PlaneProject = pm.PlaneProject
		}
	}
	return &s
}

// clone checks remote out at dir on ref (a branch or tag), creating the parent directory.
func clone(ctx context.Context, repo *vcs.RepoSync, remote, ref, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	return repo.Clone(ctx, remote, ref, dir)
}

// ensureProjectID writes the member's identity into a checkout that lacks one (a branch
// cut before the identity file was committed), and refuses a checkout of another project.
// A written file is excluded locally so it neither shows as untracked nor blocks switching
// to a branch that commits it.
func ensureProjectID(ctx context.Context, repo *vcs.RepoSync, dir, want string) error {
	got, err := projectid.Read(dir)
	switch {
	case err == nil && got == want:
		return nil
	case err == nil:
		return fmt.Errorf("%s holds project %s, not %s", dir, got, want)
	case !errors.Is(err, projectid.ErrNoProjectID):
		return err
	}
	if err := repo.Exclude(ctx, dir, "/.dendrite/"+projectid.FileName); err != nil {
		return fmt.Errorf("excluding the identity file in %s: %w", dir, err)
	}
	return fsutil.WriteFileAtomic(projectid.Path(dir), []byte(want+"\n"))
}
