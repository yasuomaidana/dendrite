package analysis

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/coreclient"
)

// sabotagingParser parses for real, then breaks the analysis the way the test asks.
type sabotagingParser struct {
	inner Parser
	after func()
}

func (p sabotagingParser) PEParse(c context.Context, path string) (*coreclient.PEInfo, error) {
	info, err := p.inner.PEParse(c, path)
	p.after()
	return info, err
}

// pausedAtParsed returns a rig with analysis id paused after its parse stage.
func pausedAtParsed(t *testing.T, id string) *rig {
	t.Helper()
	r := newRig(t)
	inner := r.Parser
	r.Parser = &pausingParser{inner: inner, svc: r.Service, id: id}
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: id}); err == nil {
		t.Fatal("expected a pause")
	}
	r.Parser = inner
	return r
}

func TestAnalyzeUsesTheServiceClock(t *testing.T) {
	r := newRig(t)
	at := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	r.Now = func() time.Time { return at }
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-clock"})
	if err != nil || !res.State.CreatedAt.Equal(at) || !res.State.UpdatedAt.Equal(at) {
		t.Errorf("%+v, %v", res.State, err)
	}
}

func TestResumeReportsDamagedCheckpoints(t *testing.T) {
	for name, damage := range map[string]func(r *rig, id string){
		"parsed structure deleted": func(r *rig, id string) { _ = os.Remove(r.infoPath(id)) },
		"parsed structure corrupt": func(r *rig, id string) { _ = os.WriteFile(r.infoPath(id), []byte("{"), 0o600) },
		"workspace path blocked": func(r *rig, id string) {
			_ = os.WriteFile(r.Store.WorkspaceDir(id), nil, 0o600)
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := pausedAtParsed(t, "pe-dmg")
			damage(r, "pe-dmg")
			res, err := r.Resume(ctx(), "pe-dmg", Options{})
			if err == nil || res.State.Error == "" || res.State.Stage != StageParsed {
				t.Errorf("%+v, %v", res, err)
			}
		})
	}
}

func TestResumeReportsAFinishedAnalysisWhoseStructureIsGone(t *testing.T) {
	r := newRig(t)
	finished(t, r, "pe-gone")
	_ = os.Remove(r.infoPath("pe-gone"))
	if _, err := r.Resume(ctx(), "pe-gone", Options{}); err == nil || !strings.Contains(err.Error(), "parsed structure is missing") {
		t.Errorf("err = %v", err)
	}
}

func TestIndexFailureIsRecordedAtTheWrittenStage(t *testing.T) {
	r := newRig(t)
	t.Setenv("STANDIN_FAIL", "index")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-idx"})
	if err == nil || res.State.Stage != StageWritten || res.State.Error == "" {
		t.Errorf("%+v, %v", res, err)
	}
}

func TestACheckpointFailsWhenTheAnalysisIsDeletedMidRun(t *testing.T) {
	r := newRig(t)
	r.Parser = sabotagingParser{inner: r.Parser, after: func() { _ = os.RemoveAll(r.Store.Dir("pe-del")) }}
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-del"}); err == nil {
		t.Error("an analysis purged while it runs must not come back")
	}
}

func TestStoresThatCannotBeWrittenAreReported(t *testing.T) {
	// parse: the structure cannot be saved
	r := newRig(t)
	stops := false
	r.Parser = sabotagingParser{inner: r.Parser, after: func() { stops = readOnly(t, r.Store.Dir("pe-ro")) }}
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-ro"}); (err != nil) != stops {
		t.Errorf("parse: permissions stop writing = %t but err = %v", stops, err)
	}

	// resume and pause: the checkpoint cannot be saved
	p := pausedAtParsed(t, "pe-ro2")
	stops = readOnly(t, p.Store.Dir("pe-ro2"))
	if _, err := p.Resume(ctx(), "pe-ro2", Options{}); (err != nil) != stops {
		t.Errorf("resume: permissions stop writing = %t but err = %v", stops, err)
	}
	if _, err := p.Pause("pe-ro2"); (err != nil) != stops {
		t.Errorf("pause: permissions stop writing = %t but err = %v", stops, err)
	}
}

// lockingIndexer indexes for real, then makes the analysis directory read-only so the checkpoint cannot be saved.
type lockingIndexer struct {
	Indexer
	lock func()
}

func (l lockingIndexer) Index(c context.Context, dir string) error {
	err := l.Indexer.Index(c, dir)
	l.lock()
	return err
}

func TestACheckpointThatCannotBeSavedStopsTheRun(t *testing.T) {
	r := newRig(t)
	stops := false
	r.Index = lockingIndexer{Indexer: r.Index, lock: func() { stops = readOnly(t, filepath.Join(r.Store.Root, "pe-lock")) }}
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "pe-lock"}); (err != nil) != stops {
		t.Errorf("permissions stop writing = %t but err = %v", stops, err)
	}
}

func TestRelativeToLeavesAPathOutsideTheWorkspaceAlone(t *testing.T) {
	if got := relativeTo("/a/work", "/a/work/imports/x.md"); got != "imports/x.md" {
		t.Errorf("inside: %q", got)
	}
	if got := relativeTo("/a/work", "/elsewhere/x.md"); got != "/elsewhere/x.md" {
		t.Errorf("outside: %q", got)
	}
	if got := relativeTo("relative", "/abs/x.md"); got != "/abs/x.md" {
		t.Errorf("unrelatable: %q", got)
	}
}
