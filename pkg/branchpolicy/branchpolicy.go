// Package branchpolicy answers one question for every Dendrite command: which branch does finished work
// in this project merge into? It replaces the "develop" that used to be written into each command.
//
// The answer comes from, in order: the synapse member's own branch (passed in as an Override, because
// pkg/synapse imports pkg/vcs and this package must stay importable from there), the project's
// [git] integration_branch directive, a local or origin "develop" (so a project with no configuration
// keeps behaving as before), the remote's default branch (origin/HEAD), main, master, and last the
// literal "develop" for a repository that has none of them yet.
package branchpolicy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// DirectivePath is where a project keeps its directives, relative to its root. pkg/directive owns the file;
// this package reads only its [git] table and cannot import pkg/directive (that would make an import
// cycle through pkg/synapse), so a test in pkg/directive keeps the two paths equal.
const DirectivePath = ".github/dendrite/directives.toml"

// DefaultFeaturePrefix starts the branch that groups the sub-tickets of a parent ticket.
const DefaultFeaturePrefix = "feature/"

// ValidBranch reports whether name is acceptable as a git branch name: non-empty, no spaces, control
// characters or any of ~^:?*[\@{, no "..", "//", leading "-" or "/", and no trailing "/", "." or ".lock".
func ValidBranch(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.HasPrefix(name, "-") ||
		strings.Contains(name, "..") || strings.Contains(name, "//") || strings.HasSuffix(name, ".lock") || strings.HasSuffix(name, ".") {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool {
		return r <= ' ' || r == 0x7f || strings.ContainsRune("~^:?*[\\@{", r)
	})
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
	SourceDevelop    Source = "existing develop branch"
	SourceRemoteHEAD Source = "remote default branch (origin/HEAD)"
	SourceMain       Source = "existing main branch"
	SourceMaster     Source = "existing master branch"
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
	// Branch is the integration branch.
	Branch string `json:"branch"`
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
	for _, b := range []string{"develop", "main", "master"} {
		if b != t.Branch {
			out = append(out, b)
		}
	}
	return out
}

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

// Resolve decides the Target of the project at dir. A malformed or invalid directive does not stop it:
// detection takes over, because a branch must always be nameable.
func Resolve(dir string, ov Override) Target {
	push := firstNonEmpty(ov.PushRemote, DefaultRemote)
	t := Target{
		PushRemote:    push,
		Remote:        firstNonEmpty(ov.TargetRemote, push),
		FeaturePrefix: DefaultFeaturePrefix,
	}
	d := readDirective(dir)
	if d.FeaturePrefix != "" {
		t.FeaturePrefix = d.FeaturePrefix
	}
	t.Branch, t.Source = chooseBranch(dir, t.Remote, ov, d)
	return t
}

func chooseBranch(dir, remote string, ov Override, d gitDirective) (string, Source) {
	if ValidBranch(ov.Branch) {
		return ov.Branch, SourceMember
	}
	if ValidBranch(d.IntegrationBranch) {
		return d.IntegrationBranch, SourceDirective
	}
	if branchExists(dir, remote, "develop") {
		return "develop", SourceDevelop
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
	return "develop", SourceDefault
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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
