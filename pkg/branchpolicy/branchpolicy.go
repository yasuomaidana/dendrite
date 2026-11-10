// Package branchpolicy answers one question for every Dendrite command: which branch does finished work
// in this project merge into? It replaces the "develop" that used to be written into each command.
//
// The answer comes from, in order: the synapse member's own branch (passed in as an Override, because
// pkg/synapse imports pkg/vcs and this package must stay importable from there), the project's
// [git] integration_branch directive, the remote's default branch (origin/HEAD), main, master, a "develop"
// that is the only one of them the repository has, and last the literal "main" for a repository that has
// none of them yet. A repository that has both main and develop integrates on main: develop is used only
// when it is named, or by the branches made before feature folders existed (see LegacyIntegration).
package branchpolicy

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// DirectivePath is where a project keeps its directives, relative to its root. pkg/directive owns the file;
// this package reads only its [git] table and cannot import pkg/directive (that would make an import
// cycle through pkg/synapse), so a test in pkg/directive keeps the two paths equal.
const DirectivePath = ".github/dendrite/directives.toml"

// DefaultFeaturePrefix starts the branch that groups the sub-tickets of a parent ticket.
const DefaultFeaturePrefix = "feature/"

// LegacyIntegration is the branch every agent's work used to merge into. A repository that still has it keeps
// it as the target of its `agent/<ticket>` and `feature/<PARENT>-<slug>` branches until it is retired.
const LegacyIntegration = "develop"

// legacyPrefixes start the branches made before feature folders: they were cut from LegacyIntegration.
var legacyPrefixes = []string{"agent/", DefaultFeaturePrefix}

// ValidBranch reports whether name is acceptable as a git branch name: non-empty, no spaces, control
// characters or any of ~^:?*[\@{, no "..", "//", leading "-" or "/", and no trailing "/", "." or ".lock".
func ValidBranch(name string) bool {
	if name == "" || hasAny(name, strings.HasPrefix, "/", "-") || hasAny(name, strings.HasSuffix, "/", ".lock", ".") || hasAny(name, strings.Contains, "..", "//") {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool {
		return r <= ' ' || r == 0x7f || strings.ContainsRune("~^:?*[\\@{", r)
	})
}

// hasAny reports whether match(s, p) holds for any of the patterns.
func hasAny(s string, match func(s, p string) bool, patterns ...string) bool {
	for _, p := range patterns {
		if match(s, p) {
			return true
		}
	}
	return false
}

// gitDirective is the [git] table of the directives file.
type gitDirective struct {
	IntegrationBranch string `toml:"integration_branch"`
	FeaturePrefix     string `toml:"feature_prefix"`
}

// readDirective reads the [git] table of the project at dir. A missing or unreadable file gives the zero
// value: the branch must always be nameable, and `dendrite directive` reports a broken file.
func readDirective(dir string) gitDirective {
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(DirectivePath)))
	if err != nil {
		return gitDirective{}
	}
	var f struct {
		Git gitDirective `toml:"git"`
	}
	if toml.Unmarshal(data, &f) != nil {
		return gitDirective{}
	}
	return f.Git
}

// Source says where a Target came from, so `dendrite branch show` can explain it.
type Source string

// The places a Target can come from, strongest first.
const (
	SourceMember     Source = "synapse member branch"
	SourceDirective  Source = "directive git.integration_branch"
	SourceTicket     Source = "ticket target recorded for this branch"
	SourceLegacy     Source = "branch made before feature folders, in a repository that still has develop"
	SourceRemoteHEAD Source = "remote default branch (origin/HEAD)"
	SourceMain       Source = "existing main branch"
	SourceMaster     Source = "existing master branch"
	SourceDevelop    Source = "existing develop branch (the repository has no main or master)"
	SourceDefault    Source = "default (no branch found yet)"
)

// DefaultRemote is the remote used when neither the project nor its synapse member names another.
const DefaultRemote = "origin"

// Override is what a synapse member declares about this project. The zero value overrides nothing.
type Override struct {
	// Branch is the integration branch for this project in the synapse.
	Branch string
	// PushRemote is where branches are pushed (a fork); empty means origin.
	PushRemote string
	// TargetRemote is the remote whose Branch work is merged or proposed against; empty means PushRemote.
	TargetRemote string
}

// Target is where finished work in a project goes.
type Target struct {
	// Branch is where finished work on the current branch merges: the project's integration branch, or the
	// feature branch of the parent ticket the branch was started for (see TargetKey).
	Branch string `json:"branch"`
	// Integration is the project's integration branch, whatever Branch is.
	Integration string `json:"integration"`
	// Remote is the remote Branch lives on (the upstream of a fork, otherwise the push remote).
	Remote string `json:"remote"`
	// PushRemote is the remote branches are pushed to.
	PushRemote string `json:"push_remote"`
	// FeaturePrefix starts the branch that groups the sub-tickets of a parent ticket.
	FeaturePrefix string `json:"feature_prefix"`
	Source        Source `json:"source"`
}

// Ref is the remote-tracking name of the integration branch, e.g. "origin/develop".
func (t Target) Ref() string { return t.Remote + "/" + t.Branch }

// Protected lists the branches no agent edits or commits on directly: the integration branch and the
// conventional trunk names.
func (t Target) Protected() []string {
	out := []string{t.Branch}
	for _, b := range []string{t.Integration, LegacyIntegration, "main", "master"} {
		if b != "" && !slices.Contains(out, b) {
			out = append(out, b)
		}
	}
	return out
}

// ByPullRequest reports whether finished work reaches Branch only by pull request: it is a trunk (main or
// master), which agents never push.
func (t Target) ByPullRequest() bool { return t.Branch == "main" || t.Branch == "master" }

// IsProtected reports whether branch is one of Protected.
func (t Target) IsProtected(branch string) bool {
	for _, p := range t.Protected() {
		if p == branch {
			return true
		}
	}
	return false
}

// MergeTargets lists the refs a work branch counts as merged into: Protected, each on its own and on the
// target remote.
func (t Target) MergeTargets() []string {
	var out []string
	for _, b := range t.Protected() {
		out = append(out, b, t.Remote+"/"+b)
	}
	return out
}

// TargetKey is the git config key (branch.<name>.dendrite-target) that records the branch finished work on
// <name> merges into when that is not the integration branch, e.g. the feature branch of a parent ticket.
func TargetKey(branch string) string { return "branch." + branch + ".dendrite-target" }

// For is Resolve with the settings of the project's synapse member (see MemberOverride), and, when the branch
// checked out in dir has a recorded target, that target in place of the integration branch.
func For(dir string) Target {
	t := Resolve(dir, MemberOverride(dir))
	if cur, ok := git(dir, "symbolic-ref", "--short", "HEAD"); ok {
		return t.withRecordedTarget(dir, cur)
	}
	return t
}

// ForBranch is For for a branch that need not be checked out in dir.
func ForBranch(dir, branch string) Target {
	return Resolve(dir, MemberOverride(dir)).withRecordedTarget(dir, branch)
}

func (t Target) withRecordedTarget(dir, branch string) Target {
	rec, ok := git(dir, "config", "--get", TargetKey(branch))
	switch {
	case ok && ValidBranch(rec):
		if rec != t.Branch {
			t.Branch, t.Source = rec, SourceTicket
		}
	case t.Branch != LegacyIntegration && hasAny(branch, strings.HasPrefix, legacyPrefixes...) && branchExists(dir, t.Remote, LegacyIntegration):
		t.Branch, t.Source = LegacyIntegration, SourceLegacy
	}
	return t
}

// Explicit reports whether a person chose the integration branch (a synapse member or the directive), rather
// than Dendrite detecting it.
func (t Target) Explicit() bool { return t.Source == SourceMember || t.Source == SourceDirective }

// Bases lists the refs to compare a branch against to see its own work, remote-tracking first:
// e.g. "origin/develop", "develop".
func (t Target) Bases() []string { return []string{t.Ref(), t.Branch} }

// Resolve decides the Target of the project at dir. A malformed or invalid directive does not stop it:
// detection takes over, because a branch must always be nameable.
func Resolve(dir string, ov Override) Target {
	push := orElse(ov.PushRemote, DefaultRemote)
	t := Target{
		PushRemote:    push,
		Remote:        orElse(ov.TargetRemote, push),
		FeaturePrefix: DefaultFeaturePrefix,
	}
	d := readDirective(dir)
	if d.FeaturePrefix != "" {
		t.FeaturePrefix = d.FeaturePrefix
	}
	t.Branch, t.Source = chooseBranch(dir, t.Remote, ov, d)
	t.Integration = t.Branch
	return t
}

func chooseBranch(dir, remote string, ov Override, d gitDirective) (string, Source) {
	if ValidBranch(ov.Branch) {
		return ov.Branch, SourceMember
	}
	if ValidBranch(d.IntegrationBranch) {
		return d.IntegrationBranch, SourceDirective
	}
	if b := remoteDefault(dir, remote); b != "" {
		return b, SourceRemoteHEAD
	}
	if branchExists(dir, remote, "main") {
		return "main", SourceMain
	}
	if branchExists(dir, remote, "master") {
		return "master", SourceMaster
	}
	if branchExists(dir, remote, LegacyIntegration) {
		return LegacyIntegration, SourceDevelop
	}
	return "main", SourceDefault
}

func orElse(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func git(dir string, args ...string) (string, bool) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err == nil
}

// branchExists reports whether the branch exists locally or on the remote.
func branchExists(dir, remote, branch string) bool {
	for _, ref := range []string{"refs/heads/" + branch, "refs/remotes/" + remote + "/" + branch} {
		if _, ok := git(dir, "rev-parse", "--verify", "--quiet", ref); ok {
			return true
		}
	}
	return false
}

// remoteDefault is the branch the remote's HEAD points to, or "" when it is not recorded.
func remoteDefault(dir, remote string) string {
	out, ok := git(dir, "symbolic-ref", "--short", "refs/remotes/"+remote+"/HEAD")
	if !ok {
		return ""
	}
	return strings.TrimPrefix(out, remote+"/")
}
