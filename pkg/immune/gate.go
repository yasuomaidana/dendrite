package immune

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/statedir"
)

// SkillRoots are the directories, relative to a repository's top, whose sub-directories are agent skills.
var SkillRoots = []string{".claude/skills", ".agents/skills"}

// DecisionAllowed marks a ledger entry that records a person's decision to accept a held scan.
const DecisionAllowed = "allowed"

// ErrNotCleared means a skill's current content has no scan in the ledger that lets it through.
var ErrNotCleared = errors.New("skill not cleared by the immune gate")

// Cleared reports whether the entry lets its content through: approved, or held with a recorded decision.
func (e Entry) Cleared() bool {
	return e.Verdict == Approve || (e.Verdict == Hold && e.Decision == DecisionAllowed)
}

// RepoTop is the nearest directory at or above path that holds a .git entry; "" when there is none.
func RepoTop(path string) string {
	dir := path
	if abs, err := filepath.Abs(path); err == nil {
		dir = abs
	}
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// LedgerFor is the scan ledger of the repository that holds path, so a scan and the hooks of that
// repository meet in one file wherever they are started from. A linked worktree shares the ledger
// of its main checkout, like the rest of .dendrite/. $DENDRITE_STATE_DIR, or a path outside any
// repository, falls back to the state directory of the current one.
func LedgerFor(path string) Ledger {
	if os.Getenv("DENDRITE_STATE_DIR") == "" {
		if top := RepoTop(path); top != "" {
			return Ledger{Path: filepath.Join(projectid.StateRoot(top), filepath.FromSlash(LedgerFile))}
		}
	}
	return Ledger{Path: statedir.Resolve(LedgerFile)}
}

// SkillOf is the skill directory the file at path belongs to (<top>/<root>/<name>), or "" when
// path is not inside a skill of the repository at top. A file lying directly in a skill root
// belongs to no skill.
func SkillOf(top, path string) string {
	rel, err := filepath.Rel(top, path)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	for _, root := range SkillRoots {
		rest, ok := strings.CutPrefix(rel, root+"/")
		if !ok {
			continue
		}
		if name, _, inside := strings.Cut(rest, "/"); inside && name != "" {
			return filepath.Join(top, filepath.FromSlash(root), name)
		}
	}
	return ""
}

// Gate decides from the ledger whether a skill's current content may be committed or built upon.
type Gate struct {
	// Pin is the scanner version the project requires; a scan made with another one does not count.
	Pin string
}

// Check is nil when the latest scan of the skill's current content clears it. Otherwise the error
// wraps ErrNotCleared, says why, and names the command that moves it forward.
func (g Gate) Check(skill string) error {
	digest, err := Digest(skill)
	if err != nil {
		return fmt.Errorf("%w: %s cannot be read: %v", ErrNotCleared, skill, err)
	}
	e, found, err := LedgerFor(skill).Find(digest)
	if err != nil {
		return fmt.Errorf("%w: the scan ledger cannot be read: %v", ErrNotCleared, err)
	}
	scan := "dendrite immune scan " + skill
	switch {
	case !found:
		return notCleared(skill, "its current content has not been scanned", scan)
	case g.Pin != "" && Version(e.ScannerVersion) != g.Pin:
		return notCleared(skill, fmt.Sprintf("it was scanned with SkillSpector %s, not the pinned %s", Version(e.ScannerVersion), g.Pin), "dendrite immune update && "+scan)
	case e.Verdict == Quarantine:
		return notCleared(skill, fmt.Sprintf("it is quarantined (score %d)", e.Score), "remove what the scan reports, then "+scan)
	case !e.Cleared():
		return notCleared(skill, fmt.Sprintf("it is held (score %d) until a person decides", e.Score), "dendrite immune allow "+skill+` --reason "<why it is safe>"`)
	}
	return nil
}

func notCleared(skill, why, remedy string) error {
	return fmt.Errorf("%w: %s: %s. Run: %s", ErrNotCleared, skill, why, remedy)
}

// CheckEdit is the pre-edit gate for a write to target: a skill that is committed at HEAD may only
// be changed while its current content is cleared, so nothing is built on content the gate has not
// seen. A new skill, and any path outside the skill roots or outside a repository, is let through;
// the commit gate judges a new skill.
func (g Gate) CheckEdit(target string) error {
	top := RepoTop(target)
	if top == "" {
		return nil
	}
	skill := SkillOf(top, target)
	if skill == "" || !committed(top, skill) {
		return nil
	}
	return g.Check(skill)
}

// CheckStaged is the pre-commit gate for the repository holding dir: every skill with a staged
// file must be cleared as it stands in the working copy. A skill that is being deleted is let
// through. Staged files that git cannot list are refused, because the commit cannot be judged.
func (g Gate) CheckStaged(dir string) error {
	top := RepoTop(dir)
	if top == "" {
		return nil
	}
	out, err := exec.Command("git", "-C", top, "diff", "--cached", "--name-only", "-z", "--diff-filter=ACMRD").Output()
	if err != nil {
		return fmt.Errorf("%w: git cannot list the staged files (%v), so the commit cannot be judged", ErrNotCleared, err)
	}
	seen := map[string]bool{}
	var errs []error
	for _, rel := range bytes.Split(out, []byte{0}) {
		skill := SkillOf(top, filepath.Join(top, string(rel)))
		if skill == "" || seen[skill] {
			continue
		}
		seen[skill] = true
		if _, err := os.Stat(skill); err == nil {
			errs = append(errs, g.Check(skill))
		}
	}
	return errors.Join(errs...)
}

// committed reports whether HEAD holds a file of the skill.
func committed(top, skill string) bool {
	rel, _ := filepath.Rel(top, skill)
	out, err := exec.Command("git", "-C", top, "ls-tree", "-r", "--name-only", "HEAD", "--", rel).Output()
	return err == nil && len(bytes.TrimSpace(out)) > 0
}

// Allow records a person's decision to accept the held scan of the skill's current content. Only a
// held scan can be decided: unscanned content has to be scanned first, quarantined content is
// never let through, and approved content needs no decision (added is false).
func Allow(skill, reason string, now time.Time) (e Entry, added bool, err error) {
	if strings.TrimSpace(reason) == "" {
		return e, false, errors.New("a decision needs a reason")
	}
	digest, err := Digest(skill)
	if err != nil {
		return e, false, fmt.Errorf("skill: %w", err)
	}
	ledger := LedgerFor(skill)
	e, found, err := ledger.Find(digest)
	switch {
	case err != nil:
		return e, false, err
	case !found:
		return e, false, fmt.Errorf("%s has not been scanned as it is now: run `dendrite immune scan %s` first", skill, skill)
	case e.Verdict == Quarantine:
		return e, false, fmt.Errorf("%s is quarantined (score %d); a quarantined skill is never let through", skill, e.Score)
	case e.Verdict == Approve:
		return e, false, nil
	}
	e.Decision, e.Reason, e.Time = DecisionAllowed, strings.TrimSpace(reason), now.UTC().Truncate(time.Second)
	added, err = ledger.Record(e)
	return e, added, err
}
