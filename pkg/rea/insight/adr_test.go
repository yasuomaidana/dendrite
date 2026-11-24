package insight

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/rea/analysis"
)

func TestDraftADRWritesAProposedRecordForAFinishedAnalysis(t *testing.T) {
	svc, id := finishedAnalysis(t, "sample.exe", "MZ")
	dir := filepath.Join(t.TempDir(), "decisions")
	path, err := DraftADR(svc, id, dir, "")
	if err != nil || path != filepath.Join(dir, "ADR-001.md") {
		t.Fatalf("%q, %v", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id: ADR-001", "status: proposed", "Cleanroom recreation of sample.exe", "an-1", "Pending", "reverse-engineering"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the ADR lacks %q:\n%s", want, data)
		}
	}
}

func TestDraftADRNumbersAfterTheHighestExistingRecord(t *testing.T) {
	svc, id := finishedAnalysis(t, "sample.exe", "MZ")
	dir := t.TempDir()
	writeFile(t, dir, "ADR-007-old-decision.md", "old")
	writeFile(t, dir, "README.md", "not a record")
	path, err := DraftADR(svc, id, dir, "A chosen title")
	if err != nil || filepath.Base(path) != "ADR-008.md" {
		t.Fatalf("%q, %v", path, err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "A chosen title") {
		t.Errorf("the title is kept:\n%s", data)
	}
	if next := nextADRID(t.TempDir()); next != "ADR-001" {
		t.Errorf("an empty directory starts at ADR-001, got %s", next)
	}
}

func TestDraftADRUsesTheProjectsDecisionsDirectoryByDefault(t *testing.T) {
	svc, id := finishedAnalysis(t, "sample.exe", "MZ")
	t.Chdir(t.TempDir())
	path, err := DraftADR(svc, id, " ", "")
	if err != nil || path != filepath.Join(DefaultADRDir, "ADR-001.md") || !exists(path) {
		t.Errorf("%q, %v", path, err)
	}
}

func TestDraftADRRefusesWhatItCannotDraft(t *testing.T) {
	svc, id := finishedAnalysis(t, "sample.exe", "MZ")
	if _, err := DraftADR(svc, "no-such", t.TempDir(), ""); !errors.Is(err, analysis.ErrNotFound) {
		t.Errorf("an unknown analysis: %v", err)
	}
	if _, err := svc.Pause(id); err == nil {
		t.Fatal("a finished analysis cannot be paused")
	}
	st, _ := svc.Store.Load(id)
	st.Stage = analysis.StageParsed
	if err := svc.Store.Save(&st); err != nil {
		t.Fatal(err)
	}
	if _, err := DraftADR(svc, id, t.TempDir(), ""); !errors.Is(err, analysis.ErrNotDone) {
		t.Errorf("an unfinished analysis: %v", err)
	}
}
