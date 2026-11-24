package analysis

import (
	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/rea"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAnalyzeRunsEveryStageAndWritesTheWorkspace(t *testing.T) {
	r := newRig(t)
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-test"})
	if err != nil {
		t.Fatal(err)
	}
	st := res.State
	if st.Stage != StageDone || st.Format != "pe" || st.Kind != "executable" || st.Tier != "local" || st.Paused || st.Binary != r.binary {
		t.Errorf("state %+v", st)
	}
	checkWorkspace(t, r.Store.WorkspaceDir("pe-test"))
	checkBundle(t, res.Bundle)
	if got, err := r.Status("pe-test"); err != nil || got.Stage != StageDone {
		t.Errorf("status %+v, %v", got, err)
	}
}

// checkWorkspace checks the documents an analysis of the stand-in image writes.
func checkWorkspace(t *testing.T, dir string) {
	t.Helper()
	var missing []string
	for _, doc := range []string{"overview.md", "sections.md", "exports.md", "libraries/kernel32.dll.md", "libraries/advapi32.dll.md"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(doc))); err != nil {
			missing = append(missing, doc)
		}
	}
	if len(missing) > 0 {
		t.Errorf("missing workspace documents: %v", missing)
	}
}

// checkBundle checks the evidence bundle is bounded and carries the imports and exports.
func checkBundle(t *testing.T, b *rea.EvidenceBundle) {
	t.Helper()
	if b == nil || b.TokenEstimate <= 0 || b.TokenEstimate > rea.MaxEvidenceTokens || b.Truncated {
		t.Fatalf("bundle %+v", b)
	}
	if len(b.CrossReferences) != 2 || b.StringConstants[0] != "ComputeChecksum" {
		t.Errorf("bundle contents %+v", b)
	}
}

// pausingParser asks for a pause while the PE structure is being read, as another process could, and counts parses.
type pausingParser struct {
	inner Inspector
	svc   *Service
	id    string
	calls int
}

func (p *pausingParser) Inspect(c context.Context, path string) (*coreclient.Image, error) {
	p.calls++
	info, err := p.inner.Inspect(c, path)
	if err == nil {
		_, err = p.svc.Pause(p.id)
	}
	return info, err
}

func TestPauseStopsAtTheStageBoundary(t *testing.T) {
	r := newRig(t)
	r.Inspector = &pausingParser{inner: r.Inspector, svc: r.Service, id: "pe-pause"}
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-pause"})
	if !errors.Is(err, ErrPaused) || res.State.Stage != StageParsed || !res.State.Paused {
		t.Fatalf("a paused analysis: %+v, %v", res, err)
	}
	if _, err := os.Stat(r.Store.WorkspaceDir("pe-pause")); !os.IsNotExist(err) {
		t.Errorf("nothing is written after the pause: %v", err)
	}
}

func TestResumeContinuesWithoutParsingAgain(t *testing.T) {
	r := pausedAtParsed(t, "pe-resume")
	p := &pausingParser{inner: r.Inspector, svc: r.Service, id: "pe-resume"}
	r.Inspector = p

	done, err := r.Resume(ctx(), "pe-resume", Options{Tier: "paid", Model: "cloud-pro"})
	if err != nil || done.State.Stage != StageDone || done.State.Paused || done.Bundle == nil {
		t.Fatalf("resume: %+v, %v", done, err)
	}
	if p.calls != 0 || done.State.Tier != "paid" || done.State.Model != "cloud-pro" {
		t.Errorf("parses %d, tier %s, model %s", p.calls, done.State.Tier, done.State.Model)
	}
}

func TestResumingAFinishedAnalysisChangesNothing(t *testing.T) {
	r := pausedAtParsed(t, "pe-twice")
	if _, err := r.Resume(ctx(), "pe-twice", Options{Model: "cloud-pro"}); err != nil {
		t.Fatal(err)
	}
	again, err := r.Resume(ctx(), "pe-twice", Options{})
	if err != nil || again.State.Model != "cloud-pro" || again.State.Stage != StageDone {
		t.Errorf("%+v, %v", again.State, err)
	}
}

func TestAFailedStageIsRecordedAndResumeRetriesIt(t *testing.T) {
	r := newRig(t)
	target := writeBinary(t, "plain text, not a PE image")
	res, err := r.Analyze(ctx(), target, Options{ID: "pe-bad", Format: "pe"})
	if err == nil || res.State.Stage != StageNew || !strings.Contains(res.State.Error, "expected a pe target") {
		t.Fatalf("a target of another format than the one expected: %+v, %v", res, err)
	}
	if err := os.WriteFile(target, []byte("MZ fixed"), 0o600); err != nil {
		t.Fatal(err)
	}
	done, err := r.Resume(ctx(), "pe-bad", Options{})
	if err != nil || done.State.Stage != StageDone || done.State.Error != "" || done.State.Format != "pe" {
		t.Errorf("resume after fixing the file: %+v, %v", done, err)
	}
}

func TestPauseAndResumeRefuseWhatTheyCannotDo(t *testing.T) {
	r := newRig(t)
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-done"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Pause("pe-done"); err == nil || !strings.Contains(err.Error(), "already finished") {
		t.Errorf("pausing a finished analysis: %v", err)
	}
	if _, err := r.Pause("pe-missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("pausing an unknown analysis: %v", err)
	}
	if _, err := r.Resume(ctx(), "pe-missing", Options{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("resuming an unknown analysis: %v", err)
	}
	if _, err := r.Resume(ctx(), "pe-done", Options{Tier: "free"}); err == nil || !strings.Contains(err.Error(), "unknown tier") {
		t.Errorf("an unknown tier: %v", err)
	}
	if _, err := r.Pause("../escape"); !errors.Is(err, ErrBadID) {
		t.Errorf("an unsafe id: %v", err)
	}
}

func TestAnalyzeValidatesItsInput(t *testing.T) {
	r := newRig(t)
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-dup"}); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		binary string
		o      Options
		want   string
	}{
		"missing file":   {filepath.Join(t.TempDir(), "none.exe"), Options{}, "is not readable"},
		"unknown format": {r.binary, Options{Format: "exe"}, "unknown format"},
		"another format": {r.binary, Options{Format: "elf"}, "expected a elf target"},
		"unknown tier":   {r.binary, Options{Tier: "free"}, "unknown tier"},
		"id in use":      {r.binary, Options{ID: "pe-dup"}, "already exists"},
		"unsafe id":      {r.binary, Options{ID: "a/b"}, "letters, digits"},
	} {
		if _, err := r.Analyze(ctx(), tc.binary, tc.o); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
	res, err := r.Analyze(ctx(), r.binary, Options{Format: "pe32"})
	if err != nil || !strings.HasPrefix(res.State.ID, "pe-") || res.State.Format != "pe" {
		t.Errorf("a generated id and the pe32 format: %+v, %v", res, err)
	}
}

// workspaceDocs returns every document of an analysis workspace by its path relative to dir.
func workspaceDocs(t *testing.T, dir string) map[string]string {
	t.Helper()
	docs := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		docs[filepath.ToSlash(rel)] = string(body)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return docs
}

// TestPauseThenResumeReproducesTheStateOfAnUninterruptedRun is the proof of AC-3 of DEN-1062: the evidence
// bundle, the final state (but for the time stamps) and every document of the workspace are the same.
func TestPauseThenResumeReproducesTheStateOfAnUninterruptedRun(t *testing.T) {
	straight := newRig(t)
	want, err := straight.Analyze(ctx(), straight.binary, Options{ID: "pe-same"})
	if err != nil {
		t.Fatal(err)
	}
	wantDocs := workspaceDocs(t, straight.Store.WorkspaceDir("pe-same"))

	paused := pausedAtParsed(t, "pe-same")
	got, err := paused.Resume(ctx(), "pe-same", Options{})
	if err != nil {
		t.Fatal(err)
	}

	// The two runs read copies of the same image from different temporary directories: only that path may differ.
	g, w := *got.Bundle, *want.Bundle
	g.TargetBinary, w.TargetBinary = "", ""
	if !reflect.DeepEqual(g, w) {
		t.Errorf("bundle after resume\n got %+v\nwant %+v", g, w)
	}
	if gs, ws := got.State, want.State; gs.Stage != ws.Stage || gs.Format != ws.Format || gs.Tier != ws.Tier || gs.Paused != ws.Paused {
		t.Errorf("state after resume %+v, uninterrupted %+v", gs, ws)
	}
	if gotDocs := workspaceDocs(t, paused.Store.WorkspaceDir("pe-same")); !reflect.DeepEqual(gotDocs, wantDocs) {
		t.Errorf("workspace after resume differs from the uninterrupted one:\n got %v\nwant %v", gotDocs, wantDocs)
	}
}
