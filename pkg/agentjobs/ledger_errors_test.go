package agentjobs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLedger_SaveFailsInReadOnlyJobDir(t *testing.T) {
	root := t.TempDir()
	l := NewLedger(root)
	job, err := l.Create(&Job{Ticket: "DEN-988"})
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	jobDirPath := l.JobDir(job.ID)
	if err := os.Chmod(jobDirPath, 0o555); err != nil {
		t.Fatalf("failed to change permissions: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(jobDirPath, 0o755) })

	if err := l.Save(job); err == nil {
		t.Errorf("expected error when saving in read-only directory")
	}
}

func TestLedger_AppendFailsWhenEventsPathIsADirectory(t *testing.T) {
	root := t.TempDir()
	l := NewLedger(root)
	job, err := l.Create(&Job{Ticket: "DEN-988"})
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	eventsPath := l.EventsPath(job.ID)
	if err := os.MkdirAll(eventsPath, 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if info, err := os.Stat(eventsPath); err != nil || !info.IsDir() {
		t.Fatalf("expected events path to be a directory, err=%v", err)
	}

	if err := l.Append(job.ID, Event{Type: "dispatched"}); err == nil {
		t.Errorf("expected error when appending to directory")
	}
}

func TestLedger_EventsOnEmptyLogReturnsEmptySlice(t *testing.T) {
	root := t.TempDir()
	l := NewLedger(root)
	job, err := l.Create(&Job{Ticket: "DEN-988"})
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	eventsPath := l.EventsPath(job.ID)
	if err := os.WriteFile(eventsPath, []byte{}, 0o644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	events, err := l.Events(job.ID)
	if err != nil || events == nil || len(events) != 0 {
		t.Errorf("expected empty slice and no error: got %v, %v", events, err)
	}
}

func TestLedger_AppendFailsWhenEventsLogIsReadOnly(t *testing.T) {
	l := NewLedger(t.TempDir())
	job, err := l.Create(&Job{Ticket: "DEN-988"})
	if err != nil {
		t.Fatalf("failed to create job: %v", err)
	}
	if err := os.WriteFile(l.EventsPath(job.ID), nil, 0o444); err != nil {
		t.Fatalf("failed to write read-only events log: %v", err)
	}
	if err := l.Append(job.ID, Event{Type: "dispatched"}); err == nil {
		t.Errorf("expected error when the events log is not writable")
	}
}

func TestWriteFileAtomic_WriteErrorKeepsDestination(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "job.json")
	if err := os.WriteFile(dest, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := writeFile
	t.Cleanup(func() { writeFile = orig })
	writeFile = func(string, []byte, os.FileMode) error { return os.ErrClosed }

	if err := writeFileAtomic(dest, []byte("new")); err == nil {
		t.Fatal("expected the write error to be returned")
	}
	if got, _ := os.ReadFile(dest); string(got) != "original" {
		t.Fatalf("destination must be untouched after a failed write, got %q", got)
	}
}

func TestWriteFileAtomic_RenameError(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "occupied")
	if err := os.MkdirAll(filepath.Join(dest, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(dest, []byte("x")); err == nil {
		t.Fatal("expected a rename error when the destination is a non-empty directory")
	}
}
