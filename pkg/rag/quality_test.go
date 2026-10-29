package rag

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/sidecar"
)

func qualityProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	body := strings.Repeat("x := 1\n", 12)
	for name, text := range map[string]string{"a.go": body, "b.go": body, "c.go": "y := 2\n", "r.rs": "REPEATED_TEST\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func useStandin(t *testing.T, protocol string) {
	t.Helper()
	bin, err := ragtest.Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sidecar.EnvVar("dendrite-rag"), bin)
	t.Setenv("STANDIN_PROTOCOL", protocol)
}

func TestQualityRunsEachGateThroughTheSidecar(t *testing.T) {
	useStandin(t, "1.11.0")
	dir := qualityProject(t)
	r := NewRetriever()
	ctx := context.Background()

	var loc struct {
		Totals struct{ Files, Failures int }
		Files  []struct {
			Path       string
			Production int
			Status     string
		}
	}
	if err := r.Quality(ctx, dir, "quality.loc", map[string]interface{}{"max_production_lines": 10, "warn_at": 5}, &loc); err != nil {
		t.Fatal(err)
	}
	if loc.Totals.Failures != 2 || loc.Totals.Files != 4 || loc.Files[0].Path != "a.go" || loc.Files[0].Status != "fail" {
		t.Errorf("loc = %+v", loc)
	}

	var clones struct {
		Totals struct{ Groups int }
		Groups []struct{ Locations []struct{ Path string } }
	}
	if err := r.Quality(ctx, dir, "quality.clones", map[string]interface{}{"min_lines": 5}, &clones); err != nil {
		t.Fatal(err)
	}
	if clones.Totals.Groups != 1 || len(clones.Groups[0].Locations) != 2 {
		t.Errorf("clones = %+v", clones)
	}

	var rst struct {
		Files      int
		Violations []struct{ Kind string }
	}
	if err := r.Quality(ctx, dir, "quality.rstest", nil, &rst); err != nil {
		t.Fatal(err)
	}
	if rst.Files != 1 || len(rst.Violations) != 1 || rst.Violations[0].Kind != "repeated_test" {
		t.Errorf("rstest = %+v", rst)
	}
}

func TestQualityRefusesWhatItCannotRunAndSaysHowToFixIt(t *testing.T) {
	ctx := context.Background()
	dir := qualityProject(t)
	if err := NewRetriever().Quality(ctx, dir, "quality.nope", nil, nil); err == nil || !strings.Contains(err.Error(), "unknown quality method") {
		t.Errorf("unknown method: %v", err)
	}
	if err := NewRetriever().Quality(ctx, filepath.Join(dir, "missing"), "quality.loc", nil, nil); err == nil {
		t.Error("a root that does not exist")
	}
	if err := NewRetriever().Quality(ctx, "", "quality.nope", nil, nil); err == nil {
		t.Error("an empty root is the current directory, and the method is still unknown")
	}

	useStandin(t, "1.10.0")
	err := NewRetriever().Quality(ctx, dir, "quality.clones", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "needs 1.11") || !strings.Contains(err.Error(), InstallHint) {
		t.Errorf("a sidecar that predates the method: %v", err)
	}
	var loc struct{ Totals struct{ Files int } }
	if err := NewRetriever().Quality(ctx, dir, "quality.loc", nil, &loc); err != nil || loc.Totals.Files != 4 {
		t.Errorf("1.10 serves quality.loc: %+v, %v", loc, err)
	}

	t.Setenv(sidecar.EnvVar("dendrite-rag"), filepath.Join(dir, "no-such-binary"))
	if err := NewRetriever().Quality(ctx, dir, "quality.loc", nil, nil); err == nil || !strings.Contains(err.Error(), "dendrite-rag") {
		t.Errorf("no sidecar: %v", err)
	}
}
