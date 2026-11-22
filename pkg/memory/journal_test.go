package memory_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"Dendrite/pkg/memory"
)

func journalIn(t *testing.T) memory.Journal {
	t.Helper()
	return memory.Journal{Path: filepath.Join(t.TempDir(), "session", "memory-journal.jsonl")}
}

func TestAppendIsReadableAtOnceAndFillsIDAndTime(t *testing.T) {
	j := journalIn(t)
	got, err := j.Append(memory.Entry{Type: "decision", Title: "use X", Body: "because Y", Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.Time.IsZero() {
		t.Fatalf("Append must fill ID and Time, got %+v", got)
	}
	pending, _, err := j.Pending()
	if err != nil || len(pending) != 1 || pending[0].ID != got.ID || pending[0].Title != "use X" {
		t.Fatalf("Pending = %+v, %v", pending, err)
	}
}

func TestDefaultJournalLivesInTheSessionStateDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DENDRITE_STATE_DIR", dir)
	if got, want := memory.DefaultJournal().Path, filepath.Join(dir, "session", "memory-journal.jsonl"); got != want {
		t.Fatalf("DefaultJournal = %q, want %q", got, want)
	}
}

func TestIDDependsOnContentNotOnTime(t *testing.T) {
	a := memory.Entry{Type: "note", Title: "t", Body: "b", Project: "p", Time: time.Unix(1, 0)}
	b := a
	b.Time = time.Unix(99, 0)
	c := a
	c.Body = "other"
	if memory.EntryID(a) != memory.EntryID(b) {
		t.Fatal("the same memory written twice must get the same ID")
	}
	if memory.EntryID(a) == memory.EntryID(c) {
		t.Fatal("different bodies must get different IDs")
	}
}

func TestAppendRejectsAnEntryWithoutTitleOrBody(t *testing.T) {
	j := journalIn(t)
	if _, err := j.Append(memory.Entry{Body: "b"}); err == nil {
		t.Fatal("an entry without a title must be refused")
	}
	if _, err := j.Append(memory.Entry{Title: "t"}); err == nil {
		t.Fatal("an entry without a body must be refused")
	}
}

func TestATornLastLineIsDroppedAndTheNextAppendSurvives(t *testing.T) {
	j := journalIn(t)
	if _, err := j.Append(memory.Entry{Title: "one", Body: "1"}); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(j.Path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"id":"torn","title":"half`); err != nil { // kill -9 mid-write: no newline
		t.Fatal(err)
	}
	_ = f.Close()

	pending, _, err := j.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatalf("torn line must be ignored: %+v, %v", pending, err)
	}
	if _, err := j.Append(memory.Entry{Title: "two", Body: "2"}); err != nil {
		t.Fatal(err)
	}
	pending, _, _ = j.Pending()
	if len(pending) != 2 || pending[1].Title != "two" {
		t.Fatalf("the append after a torn line must not be glued to it: %+v", pending)
	}
}

func TestAGarbledMiddleLineIsSkipped(t *testing.T) {
	j := journalIn(t)
	if _, err := j.Append(memory.Entry{Title: "one", Body: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.Path, append(mustRead(t, j.Path), []byte("not json\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Append(memory.Entry{Title: "two", Body: "2"}); err != nil {
		t.Fatal(err)
	}
	pending, _, _ := j.Pending()
	if len(pending) != 2 {
		t.Fatalf("garbled line must be skipped, entries kept: %+v", pending)
	}
}

func TestMarkFlushedHidesEntriesUpToTheOffset(t *testing.T) {
	j := journalIn(t)
	_, _ = j.Append(memory.Entry{Title: "one", Body: "1"})
	_, end, _ := j.Pending()
	if err := j.MarkFlushed(end); err != nil {
		t.Fatal(err)
	}
	if pending, _, _ := j.Pending(); len(pending) != 0 {
		t.Fatalf("flushed entries must not be pending: %+v", pending)
	}
	_, _ = j.Append(memory.Entry{Title: "two", Body: "2"})
	pending, _, _ := j.Pending()
	if len(pending) != 1 || pending[0].Title != "two" {
		t.Fatalf("only the new entry is pending: %+v", pending)
	}
}

func TestPendingOnAMissingJournalIsEmpty(t *testing.T) {
	pending, end, err := journalIn(t).Pending()
	if err != nil || len(pending) != 0 || end != 0 {
		t.Fatalf("missing journal: %+v %d %v", pending, end, err)
	}
}

func TestAnUnreadableOffsetFileStartsFromZero(t *testing.T) {
	j := journalIn(t)
	_, _ = j.Append(memory.Entry{Title: "one", Body: "1"})
	if err := os.WriteFile(j.Path+".flushed", []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if pending, _, _ := j.Pending(); len(pending) != 1 {
		t.Fatalf("a bad offset must replay, never lose: %+v", pending)
	}
}

func TestAppendFailsWhenThePathCannotBeWritten(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(file, nil, 0o644)
	j := memory.Journal{Path: filepath.Join(file, "child", "j.jsonl")}
	if _, err := j.Append(memory.Entry{Title: "t", Body: "b"}); err == nil {
		t.Fatal("expected an error")
	}
	dir := memory.Journal{Path: t.TempDir()}
	if _, _, err := dir.Pending(); err == nil {
		t.Fatal("reading a directory as a journal must fail")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
