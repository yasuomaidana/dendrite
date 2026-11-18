package hook

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/devtool"
	"Dendrite/pkg/directive"
	"Dendrite/pkg/immune"
	"Dendrite/pkg/testkit"
	"context"
	"errors"
	"path/filepath"
	"testing"
)

const hookSkillHead = "---\nname: fixture\ndescription: fixture skill\n---\n"

// skillRig is a claimed working copy with one committed skill, "known", and a way to scan a skill
// into the ledger the hooks read. The scanner stand-in is SkillSpector 2.12.0 and approves everything.
type skillRig struct {
	*hookRig
	known string
	bin   string
}

func newSkillRig(t *testing.T) *skillRig {
	t.Helper()
	r := newHookRig(t, "In Progress")
	r.claim()
	known := filepath.Join(r.repo, ".claude", "skills", "known")
	testkit.WriteFile(t, filepath.Join(known, "SKILL.md"), hookSkillHead+"Format the date.\n", 0o644)
	testkit.Git(t, r.repo, "add", "-A")
	testkit.Git(t, r.repo, "commit", "-q", "-m", "a skill")
	bin := filepath.Join(t.TempDir(), "skillspector")
	testkit.WriteFile(t, bin, `#!/bin/sh
if [ "$1" = "--version" ]; then echo "SkillSpector v2.12.0"; exit 0; fi
echo '{"skill":{"name":"s"},"risk_assessment":{"score":0,"severity":"LOW"},"issues":[]}' > "$6"
`, 0o755)
	return &skillRig{hookRig: r, known: known, bin: bin}
}

func (r *skillRig) scan(skill string) {
	r.t.Helper()
	ledger := immune.LedgerFor(skill)
	_, err := immune.Scanner{Bin: r.bin, Ledger: &ledger}.Scan(context.Background(), skill)
	testkit.Check(r.t, err)
}

// blocked requires err to be the exit-2 refusal of the skill gate, holding every want.
func blocked(t *testing.T, err error, wants ...string) {
	t.Helper()
	var usage *cli.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("want a refusal (usage error, exit 2), got %v", err)
	}
	testkit.Contains(t, err.Error(), append([]string{"blocked: skill not cleared"}, wants...)...)
}

// pre-edit refuses to change a committed skill whose current content has no cleared scan, and
// lets a new skill and everything else through.
func TestPreEditGuardsCommittedSkills(t *testing.T) {
	r := newSkillRig(t)
	ctx := context.Background()
	target := filepath.Join(r.known, "SKILL.md")
	blocked(t, runPreEdit(ctx, r.event("Edit", target, ""), r.deps), "has not been scanned", "dendrite immune scan "+r.known)

	r.scan(r.known)
	testkit.Check(t, runPreEdit(ctx, r.event("Edit", target, ""), r.deps))
	testkit.Check(t, runPreEdit(ctx, r.event("Write", filepath.Join(r.repo, ".claude", "skills", "fresh", "SKILL.md"), ""), r.deps))
	testkit.Check(t, runPreEdit(ctx, r.event("Write", filepath.Join(r.repo, "pkg", "a.go"), ""), r.deps))
	testkit.Check(t, runPreEdit(ctx, r.event("ExitPlanMode", "", "plan"), r.deps))
}

// pre-commit refuses a staged skill until its current content is cleared; the ticket guard's
// bypass does not switch the skill gate off.
func TestPreCommitNeedsAClearedScanOfStagedSkills(t *testing.T) {
	r := newSkillRig(t)
	ctx := context.Background()
	fresh := filepath.Join(r.repo, ".agents", "skills", "fresh")
	testkit.WriteFile(t, filepath.Join(fresh, "SKILL.md"), hookSkillHead+"Format the date.\n", 0o644)
	testkit.Git(t, r.repo, "add", "-A")
	blocked(t, runPreCommit(ctx, r.deps, r.repo), "has not been scanned", "dendrite immune scan "+fresh)

	t.Setenv(devtool.TicketGuardEnv, "off")
	blocked(t, runPreCommit(ctx, r.deps, r.repo), "has not been scanned")
	t.Setenv(devtool.TicketGuardEnv, "")

	r.scan(fresh)
	testkit.Check(t, runPreCommit(ctx, r.deps, r.repo))

	testkit.WriteFile(t, filepath.Join(fresh, "SKILL.md"), hookSkillHead+"Something else.\n", 0o644)
	blocked(t, runPreCommit(ctx, r.deps, r.repo), "has not been scanned")
}

// The scanner version the directives pin applies to the hooks; directives that cannot be read pin nothing.
func TestSkillGateUsesThePinOfTheDirectives(t *testing.T) {
	r := newSkillRig(t)
	ctx := context.Background()
	r.scan(r.known)
	target := filepath.Join(r.known, "SKILL.md")
	directives := filepath.Join(r.repo, filepath.FromSlash(directive.Path))

	testkit.WriteFile(t, directives, "schema = 1\n[immune]\nscanner_version = \"2.13.0\"\n", 0o644)
	blocked(t, runPreEdit(ctx, r.event("Edit", target, ""), r.deps), "not the pinned 2.13.0", "dendrite immune update")

	testkit.WriteFile(t, directives, "schema = 1\n[immune]\nscanner_version = \"2.12.0\"\n", 0o644)
	testkit.Check(t, runPreEdit(ctx, r.event("Edit", target, ""), r.deps))

	testkit.WriteFile(t, directives, "not toml [", 0o644)
	testkit.Check(t, runPreEdit(ctx, r.event("Edit", target, ""), r.deps))
}
