package analysis

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/rea"
)

func TestAnalyzeRunsEveryStageAndWritesTheWorkspace(t *testing.T) {
	r := newRig(t)
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-test"})
	if err != nil {
		t.Fatal(err)
	}
	st := res.State
	if st.Stage != StageDone || st.Format != "pe64" || st.Tier != "local" || st.Paused || st.Binary != r.binary {
		t.Errorf("state %+v", st)
	}
	for _, doc := range []string{"overview.md", "sections.md", "exports.md", "imports/kernel32.dll.md", "imports/advapi32.dll.md"} {
		if _, err := os.Stat(filepath.Join(r.Store.WorkspaceDir("pe-test"), filepath.FromSlash(doc))); err != nil {
			t.Errorf("workspace document %s: %v", doc, err)
		}
	}
	b := res.Bundle
	if b == nil || b.TokenEstimate <= 0 || b.TokenEstimate > rea.MaxEvidenceTokens || b.Truncated || len(b.CrossReferences) != 2 || b.StringConstants[0] != "ComputeChecksum" {
		t.Errorf("bundle %+v", b)
	}
	if got, err := r.Status("pe-test"); err != nil || got.Stage != StageDone {
		t.Errorf("status %+v, %v", got, err)
	}
}

// pausingParser asks for a pause while the PE structure is being read, as another process could, and counts parses.
type pausingParser struct {
	inner Parser
	svc   *Service
	id    string
	calls int
}

func (p *pausingParser) PEParse(c context.Context, path string) (*coreclient.PEInfo, error) {
	p.calls++
	info, err := p.inner.PEParse(c, path)
	if err == nil {
		_, err = p.svc.Pause(p.id)
	}
	return info, err
}

func TestPauseStopsAtTheStageBoundaryAndResumeContinuesWithoutParsingAgain(t *testing.T) {
	r := newRig(t)
	p := &pausingParser{inner: r.Parser, svc: r.Service, id: "pe-pause"}
	r.Parser = p

	res, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-pause"})
	if !errors.Is(err, ErrPaused) || res.State.Stage != StageParsed || !res.State.Paused {
		t.Fatalf("a paused analysis: %+v, %v", res, err)
	}
	if _, err := os.Stat(r.Store.WorkspaceDir("pe-pause")); !os.IsNotExist(err) {
		t.Errorf("nothing is written after the pause: %v", err)
	}

	done, err := r.Resume(ctx(), "pe-pause", Options{Tier: "paid", Model: "cloud-pro"})
	if err != nil || done.State.Stage != StageDone || done.State.Paused || done.Bundle == nil {
		t.Fatalf("resume: %+v, %v", done, err)
	}
	if p.calls != 1 || done.State.Tier != "paid" || done.State.Model != "cloud-pro" {
		t.Errorf("parses %d, tier %s, model %s", p.calls, done.State.Tier, done.State.Model)
	}

	again, err := r.Resume(ctx(), "pe-pause", Options{})
	if err != nil || again.State.Model != "cloud-pro" || p.calls != 1 {
		t.Errorf("resuming a finished analysis changes nothing: %+v, %v", again.State, err)
	}
}

func TestAFailedStageIsRecordedAndResumeRetriesIt(t *testing.T) {
	r := newRig(t)
	notPE := writeBinary(t, "ELF")
	res, err := r.Analyze(ctx(), notPE, Options{ID: "pe-bad"})
	if !errors.Is(err, coreclient.ErrNotPE) || res.State.Stage != StageNew || !strings.Contains(res.State.Error, "not a PE file") {
		t.Fatalf("a file that is not a PE image: %+v, %v", res, err)
	}
	if err := os.WriteFile(notPE, []byte("MZ fixed"), 0o600); err != nil {
		t.Fatal(err)
	}
	done, err := r.Resume(ctx(), "pe-bad", Options{})
	if err != nil || done.State.Stage != StageDone || done.State.Error != "" {
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
		"missing file":   {filepath.Join(t.TempDir(), "none.exe"), Options{}, "not a readable file"},
		"a directory":    {t.TempDir(), Options{}, "not a readable file"},
		"unknown format": {r.binary, Options{Format: "elf"}, "unknown format"},
		"unknown tier":   {r.binary, Options{Tier: "free"}, "unknown tier"},
		"id in use":      {r.binary, Options{ID: "pe-dup"}, "already exists"},
		"unsafe id":      {r.binary, Options{ID: "a/b"}, "letters, digits"},
	} {
		if _, err := r.Analyze(ctx(), tc.binary, tc.o); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
	res, err := r.Analyze(ctx(), r.binary, Options{Format: "pe32"})
	if err != nil || !strings.HasPrefix(res.State.ID, "pe-") || res.State.Format != "pe32" {
		t.Errorf("a generated id and the pe32 format: %+v, %v", res, err)
	}
}
