package repair

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"Dendrite/pkg/pool/switcher"
	"Dendrite/pkg/pullrequest"
)

// Step is what Advance did, or could not do, for one link.
type Step struct {
	ID   string `json:"id"`
	From Stage  `json:"from"`
	To   Stage  `json:"to"`
	// Note says what changed in the consumer, or why the link stays where it is.
	Note string `json:"note"`
}

// Moved reports whether the step changed the link's stage.
func (s Step) Moved() bool { return s.From != s.To }

// Advance moves every link as far as it can go, the links at the bottom of a chain first:
//
//	repairing -> merged    the fix's pull request was merged: the consumer is pinned to the dependency's
//	                       repository at the merge commit, and the repair branch is no longer needed
//	merged    -> released  a tag of the dependency holds the merge commit: a consumer that used a published
//	                       version gets that release, any other gets back exactly what it had
//
// A link moves only when the links below it have at least merged, because its own fix builds on theirs.
func Advance(ctx context.Context, ledger *Ledger) ([]Step, error) {
	links, err := ledger.Links()
	if err != nil {
		return nil, err
	}
	stage := map[string]Stage{}
	for _, k := range links {
		stage[k.ID] = k.Stage
	}
	var steps []Step
	var changed []Link
	for _, k := range LeafFirst(links) {
		if k.Stage == StageReleased {
			continue
		}
		step := Step{ID: k.ID, From: k.Stage, To: k.Stage}
		if waiting := repairingBelow(links, k.ID, stage); waiting != "" {
			step.Note = "waits for " + waiting + " to be merged"
		} else if next, note, err := k.advance(ctx); err != nil {
			step.Note = err.Error()
		} else {
			step.To, step.Note = next.Stage, note
			stage[k.ID] = next.Stage
			changed = append(changed, next)
		}
		steps = append(steps, step)
	}
	return steps, ledger.Put(changed...)
}

// repairingBelow names a link below id that is still being repaired, or "".
func repairingBelow(links []Link, id string, stage map[string]Stage) string {
	for _, child := range Children(links, id) {
		if stage[child.ID] == StageRepairing {
			return child.ID
		}
	}
	return ""
}

// advance takes one step for the link and returns it as it is afterwards, with a note for the person.
func (k Link) advance(ctx context.Context) (Link, string, error) {
	if _, err := os.Stat(k.Consumer); err != nil {
		return k, "", fmt.Errorf("the consumer %s is gone: restore its manifest by hand, then `dendrite repair drop %s`", k.Consumer, k.ID)
	}
	if k.Stage == StageRepairing {
		return k.toMerged(ctx)
	}
	return k.toReleased(ctx)
}

func (k Link) toMerged(ctx context.Context) (Link, string, error) {
	pr, err := pullrequest.View(ctx, k.Copy, k.Branch)
	if errors.Is(err, pullrequest.ErrNone) {
		return k, "", errors.New("no pull request yet: `dendrite repair pr`")
	}
	if err != nil {
		return k, "", err
	}
	k.PullRequest = pr.URL
	if !pr.Merged() {
		return k, "the pull request is not merged yet: " + pr.URL, nil
	}
	if pr.MergeCommit == "" {
		return k, "", errors.New("the pull request is merged but reports no merge commit")
	}
	req := switcher.SwitchRequest{TargetProjectDir: k.Consumer, DependencyName: k.Dependency, Mode: switcher.ModeBranch, ForkURL: k.Upstream, BranchName: pr.Base, CommitSHA: pr.MergeCommit}
	if _, err := switcher.NewDependencySwitcher().Switch(ctx, req); err != nil {
		return k, "", fmt.Errorf("pinning %s to %s: %w", k.Dependency, pr.MergeCommit, err)
	}
	k.Stage, k.MergeSHA = StageMerged, pr.MergeCommit
	return k, fmt.Sprintf("merged as %s: the consumer is pinned to %s at that commit (commit the manifest); the branch %s can go", short(pr.MergeCommit), k.Upstream, k.Branch), nil
}

func (k Link) toReleased(ctx context.Context) (Link, string, error) {
	tag, err := releaseHolding(ctx, k)
	if err != nil {
		return k, "", err
	}
	if tag == "" {
		return k, "merged, and no release holds " + short(k.MergeSHA) + " yet", nil
	}
	// A published switch rewrites every manifest, so it goes first; the entries that named the dependency another
	// way are then put back exactly as they were.
	for _, e := range k.Original {
		if e.Mode == switcher.ModePublished {
			req := switcher.SwitchRequest{TargetProjectDir: k.Consumer, DependencyName: k.Dependency, Mode: switcher.ModePublished, Version: versionFor(e.Manifest, tag)}
			if _, err := switcher.NewDependencySwitcher().Switch(ctx, req); err != nil {
				return k, "", fmt.Errorf("moving %s to %s: %w", k.Dependency, tag, err)
			}
			break
		}
	}
	for _, e := range k.Original {
		if e.Mode != switcher.ModePublished {
			if err := switcher.Restore(k.Consumer, k.Dependency, e); err != nil {
				return k, "", fmt.Errorf("restoring %s in %s: %w", k.Dependency, e.Manifest, err)
			}
		}
	}
	k.Stage, k.Version = StageReleased, tag
	return k, "released as " + tag + ": the consumer names " + k.Dependency + " the normal way again (commit the manifest)", nil
}

// versionFor is the version a manifest wants for a release tag: Go keeps the v, the others drop it.
func versionFor(manifest, tag string) string {
	if manifest == "go.mod" {
		return tag
	}
	return strings.TrimPrefix(tag, "v")
}

func short(sha string) string { return sha[:min(9, len(sha))] }

// releaseHolding is the lowest version tag of the dependency's repository that contains the link's merge
// commit, or "". It looks in the working copy when it is still there, else in a throw-away copy without files.
func releaseHolding(ctx context.Context, k Link) (string, error) {
	dir, remote := k.Copy, k.upstreamRemote()
	if _, err := os.Stat(dir); err != nil {
		tmp, _ := os.MkdirTemp("", "dendrite-repair-") // a failure here shows as a failed clone
		defer os.RemoveAll(tmp)
		if _, err := Git(ctx, tmp, "clone", "-q", "--bare", "--filter=blob:none", k.Upstream, "."); err != nil {
			return "", fmt.Errorf("looking for a release of %s: %w", k.Dependency, err)
		}
		dir = tmp
	} else if _, err := Git(ctx, dir, "fetch", "-q", "--tags", remote); err != nil {
		return "", fmt.Errorf("looking for a release of %s: %w", k.Dependency, err)
	}
	tags, err := Git(ctx, dir, "tag", "--contains", k.MergeSHA, "--sort=version:refname")
	if err != nil {
		return "", fmt.Errorf("looking for a release of %s: %w", k.Dependency, err)
	}
	first, _, _ := strings.Cut(tags, "\n")
	return strings.TrimSpace(first), nil
}

// AdvanceQuietly advances the repairs of the Dendrite home after another command and writes one line per link
// that moved. It never fails the caller: a problem is written as a warning.
func AdvanceQuietly(ctx context.Context, out io.Writer) {
	ledger, err := Open()
	var steps []Step
	if err == nil {
		steps, err = Advance(ctx, ledger)
	}
	if err != nil {
		fmt.Fprintf(out, "[!] Repairs not advanced: %v\n", err)
	}
	for _, s := range steps {
		if s.Moved() {
			fmt.Fprintln(out, s.Line())
		}
	}
}

// Line is the step as one line of output.
func (s Step) Line() string {
	if s.Moved() {
		return fmt.Sprintf("[✓] %s: %s -> %s; %s", s.ID, s.From, s.To, s.Note)
	}
	return fmt.Sprintf("[=] %s: %s; %s", s.ID, s.From, s.Note)
}
