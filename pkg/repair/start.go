package repair

// TODO(move:dendrite-git-rs,DGIT-7): cloning a repository and pushing a branch is repository plumbing that
//   dendrite-git owns (git.worktree.create is DGIT-7; clone and push have no method yet).
//   When: the sidecar exposes clone and push; then the git calls here go through it.
//   Stays in Go: naming the repair branch, the ledger and switching the consumer's manifest.
//   Until then: keep these calls thin; add no git features here.

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"Dendrite/pkg/featurebranch"
	"Dendrite/pkg/pool/switcher"
	"Dendrite/pkg/workcopy"
)

// StartOptions says which dependency of which consumer is repaired, and where its code comes from.
type StartOptions struct {
	// Consumer is the directory of the project that uses the dependency.
	Consumer string
	// Dependency is the name the consumer's manifests use.
	Dependency string
	// Ticket is the consumer's ticket; it names the link and, without DepTicket, the repair branch.
	Ticket string
	// DepTicket is the dependency project's own ticket for the fix; when set it names the repair branch, so the
	// dependency's edit lock accepts a claim on it.
	DepTicket string
	// Checkout is a local checkout of the dependency we may push from: the repair copy is a worktree of it.
	// Empty clones Fork (or Upstream) instead.
	Checkout string
	// Upstream is the URL of the dependency's repository (default: the origin of Checkout).
	Upstream string
	// Fork is the URL of a fork to push the repair branch to, for a repository we cannot push.
	Fork string
	// Now stamps the link (default time.Now).
	Now func() time.Time
}

// Started is a new link and what could not be done for it.
type Started struct {
	Link Link
	// Warnings are steps that failed without stopping the repair, e.g. a push with no network.
	Warnings []string
}

// Git runs git in dir and returns what it printed; a failure carries the first line of its output.
func Git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		line, _, _ := strings.Cut(text, "\n")
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), line)
	}
	return text, nil
}

// ticketLead is the ticket id that starts a branch segment such as DEN-890-fix-login-crash.
var ticketLead = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[0-9]+-`)

// BranchName is the repair branch: the trunk `<ticket>-<slug>/main` of the dependency's ticket when there is
// one, else of the consumer's ticket. The slug is the one of the consumer's own branch, so every branch of a
// chain reads alike.
func BranchName(consumerBranch, ticket, depTicket string) string {
	segments := strings.Split(strings.TrimSuffix(consumerBranch, "/"+featurebranch.TrunkLeaf), "/")
	last := segments[len(segments)-1]
	slug := ""
	if lead := ticketLead.FindString(last); lead != "" {
		slug = strings.ToLower(strings.TrimPrefix(last, lead))
	}
	owner := cmp.Or(depTicket, ticket)
	if slug == "" {
		return owner + "/" + featurebranch.TrunkLeaf
	}
	return owner + "-" + slug + "/" + featurebranch.TrunkLeaf
}

// Start begins a repair: it makes a working copy of the dependency on a repair branch, pushes that branch,
// points the consumer's manifests at it and records the link. A consumer that is itself the working copy of a
// repair makes the new link a child of that one.
func Start(ctx context.Context, ledger *Ledger, o StartOptions) (Started, error) {
	link, err := newLink(ctx, ledger, o)
	if err != nil {
		return Started{}, err
	}
	if link.Copy, err = makeCopy(ctx, link.Consumer, o, link.Branch); err != nil {
		return Started{}, err
	}
	link.Remote, _ = Git(ctx, link.Copy, "remote", "get-url", "origin")
	link.Upstream = cmp.Or(link.Upstream, link.Remote)
	res := Started{}
	if _, err := Git(ctx, link.Copy, "push", "-u", "origin", link.Branch); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s was not pushed (%v): push it before others build %s", link.Branch, err, filepath.Base(link.Consumer)))
	}
	if err := pointAt(ctx, link); err != nil {
		return Started{}, fmt.Errorf("repair: the copy is at %s but %s was not switched: %w", link.Copy, filepath.Base(link.Consumer), err)
	}
	link.StartedAt = time.Now().UTC()
	if o.Now != nil {
		link.StartedAt = o.Now().UTC()
	}
	res.Link = link
	return res, ledger.Put(link)
}

// check reports what the options lack.
func (o StartOptions) check() error {
	if o.Dependency == "" || o.Ticket == "" {
		return errors.New("repair: a dependency and the consumer's ticket are required")
	}
	if o.Checkout == "" && o.Upstream == "" && o.Fork == "" {
		return fmt.Errorf("repair: where does %s live? give its repository URL, a fork, or a local checkout", o.Dependency)
	}
	return nil
}

// newLink checks that the repair can start and describes it: the consumer, how it names the dependency now, the
// repair branch, and the link whose working copy the consumer is. Nothing is changed yet.
func newLink(ctx context.Context, ledger *Ledger, o StartOptions) (Link, error) {
	if err := o.check(); err != nil {
		return Link{}, err
	}
	abs, _ := filepath.Abs(o.Consumer)
	link := Link{
		ID: o.Ticket + "/" + o.Dependency, Consumer: workcopy.Canon(abs), Ticket: o.Ticket, Dependency: o.Dependency, DepTicket: o.DepTicket,
		Upstream: o.Upstream, Fork: o.Fork != "", Stage: StageRepairing,
	}
	links, err := ledger.Links()
	if err != nil {
		return Link{}, err
	}
	for _, k := range links {
		if k.ID == link.ID {
			return Link{}, fmt.Errorf("repair: %s is being repaired for %s already (`dendrite repair status`)", o.Dependency, o.Ticket)
		}
		if k.Copy == link.Consumer {
			link.Parent = k.ID
		}
	}
	if link.Original, err = switcher.Current(link.Consumer, o.Dependency); err != nil {
		return Link{}, err
	}
	consumerBranch, _ := Git(ctx, link.Consumer, "symbolic-ref", "--short", "HEAD")
	link.Branch = BranchName(consumerBranch, o.Ticket, o.DepTicket)
	return link, nil
}

// makeCopy creates the working copy of the dependency on branch, next to the consumer's own copies: a worktree
// of o.Checkout when we have one, otherwise a clone of the fork (or of the upstream).
func makeCopy(ctx context.Context, consumer string, o StartOptions, branch string) (string, error) {
	name := strings.TrimSuffix(branch, "/"+featurebranch.TrunkLeaf)
	if o.Checkout != "" {
		return workcopy.New(o.Checkout).Create(ctx, name, branch, "")
	}
	root, err := workcopy.New(consumer).Root(ctx)
	if err != nil {
		return "", err
	}
	source := cmp.Or(o.Fork, o.Upstream)
	repo := strings.TrimSuffix(path.Base(filepath.ToSlash(source)), ".git")
	dest := filepath.Join(root, workcopy.SanitizeName(repo+"-"+name))
	for _, args := range [][]string{{"clone", "-q", source, dest}, {"-C", dest, "checkout", "-q", "-b", branch}} {
		if _, err := Git(ctx, filepath.Dir(consumer), args...); err != nil {
			return "", fmt.Errorf("repair: %w", err)
		}
	}
	if o.Fork != "" && o.Upstream != "" {
		_, _ = Git(ctx, dest, "remote", "add", "upstream", o.Upstream)
	}
	return workcopy.Canon(dest), nil
}

// pointAt switches the consumer's manifests to the repair branch. A Go module cannot name a branch of another
// repository without a resolved version, so go.mod is pointed at the working copy instead.
func pointAt(ctx context.Context, link Link) error {
	consumer := link.Consumer
	req := switcher.SwitchRequest{TargetProjectDir: consumer, DependencyName: link.Dependency, Mode: switcher.ModeFork, ForkURL: link.Remote, ForkBranch: link.Branch}
	for _, e := range link.Original {
		if e.Manifest == "go.mod" {
			req = switcher.SwitchRequest{TargetProjectDir: consumer, DependencyName: link.Dependency, Mode: switcher.ModeLocal, LocalPath: link.Copy}
		}
	}
	// A local switch also lists the copy in go.work, where it would outlive the repair and break the build once
	// the copy is disposed of; the replace line in go.mod is enough, so go.work is left as it was.
	goWork := filepath.Join(consumer, "go.work")
	before, readErr := os.ReadFile(goWork)
	_, err := switcher.NewDependencySwitcher().Switch(ctx, req)
	if readErr != nil {
		_ = os.Remove(goWork)
	} else {
		_ = os.WriteFile(goWork, before, 0o644)
	}
	return err
}
