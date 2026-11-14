package quality

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/quality/coverage"
	"context"
	"errors"
	"strings"
	"testing"
)

func useCoverage(t *testing.T, fn func(context.Context, coverage.Options) (*coverage.Result, error)) {
	t.Helper()
	prev := runCoverage
	runCoverage = fn
	t.Cleanup(func() { runCoverage = prev })
}

func TestAllostasisCoverage(t *testing.T) {
	var got coverage.Options
	useCoverage(t, func(_ context.Context, o coverage.Options) (*coverage.Result, error) {
		got = o
		return &coverage.Result{Passed: true}, nil
	})
	if _, code := cliOutput(t, "allostasis", "coverage", "--ratchet", "--update", "--baseline", "b.json", "pkg/a", "pkg/b"); code != cli.ExitOK {
		t.Fatalf("coverage: exit %d", code)
	}
	if !got.Ratchet || !got.Update || got.BaselinePath != "b.json" || strings.Join(got.Packages, ",") != "pkg/a,pkg/b" {
		t.Fatalf("options not passed through: %+v", got)
	}
	if _, code := cliOutput(t, "quality", "coverage", "--bogus"); code != cli.ExitUsage {
		t.Fatalf("unknown flag: exit %d", code)
	}

	useCoverage(t, func(context.Context, coverage.Options) (*coverage.Result, error) {
		return &coverage.Result{Passed: false}, nil
	})
	if out, code := cliOutput(t, "allostasis", "coverage", "--ratchet"); code != cli.ExitError || !strings.Contains(out, "coverage ratchet failed") {
		t.Fatalf("ratchet failure: exit %d\n%s", code, out)
	}
	useCoverage(t, func(context.Context, coverage.Options) (*coverage.Result, error) {
		return nil, errors.New("go test crashed")
	})
	if out, code := cliOutput(t, "allostasis", "coverage"); code != cli.ExitError || !strings.Contains(out, "go test crashed") {
		t.Fatalf("run error: exit %d\n%s", code, out)
	}
}

func TestAllostasisAudit(t *testing.T) {
	t.Chdir("../../..")
	out, code := cliOutput(t, "allostasis", "audit", "pkg/statedir")
	if code != cli.ExitOK || !strings.Contains(out, "Statement Coverage") {
		t.Fatalf("audit: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "allostasis", "audit", "pkg/does-not-exist"); code == cli.ExitOK {
		t.Fatal("auditing a missing package should fail")
	}
}
