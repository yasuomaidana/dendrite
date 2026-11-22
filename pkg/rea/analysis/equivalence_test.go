package analysis

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

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
