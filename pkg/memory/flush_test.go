package memory_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"Dendrite/pkg/engram"
	"Dendrite/pkg/memory"
	"Dendrite/pkg/testkit"
)

func fill(t *testing.T, j memory.Journal, titles ...string) {
	t.Helper()
	for _, title := range titles {
		if _, err := j.Append(memory.Entry{Type: "note", Title: title, Body: "body of " + title}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFlushSavesEveryPendingEntryOnceAndMarksThemFlushed(t *testing.T) {
	j := journalIn(t)
	fill(t, j, "one", "two")
	var saved []string
	save := func(_ context.Context, e memory.Entry) error { saved = append(saved, e.Title); return nil }

	n, err := memory.Flush(context.Background(), j, save)
	testkit.Check(t, err)
	testkit.Equal(t, "flushed", n, 2)

	n, err = memory.Flush(context.Background(), j, save)
	testkit.Check(t, err)
	testkit.Equal(t, "second flush adds nothing", n, 0)
	testkit.Equal(t, "saves", strings.Join(saved, ","), "one,two")
}

func TestAFailedSaveKeepsTheJournalPendingSoTheNextFlushRetries(t *testing.T) {
	j := journalIn(t)
	fill(t, j, "one", "two")
	boom := errors.New("engram down")
	n, err := memory.Flush(context.Background(), j, func(_ context.Context, e memory.Entry) error {
		if e.Title == "two" {
			return boom
		}
		return nil
	})
	if !errors.Is(err, boom) || n != 1 {
		t.Fatalf("Flush = %d, %v; want 1 saved and the error", n, err)
	}
	if pending, _, _ := j.Pending(); len(pending) != 2 {
		t.Fatalf("nothing may be marked flushed after a failure: %+v", pending)
	}
}

func TestFlushReportsAnUnreadableJournal(t *testing.T) {
	_, err := memory.Flush(context.Background(), memory.Journal{Path: t.TempDir()}, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestFlushReportsAnUnwritableOffset(t *testing.T) {
	j := journalIn(t)
	fill(t, j, "one")
	if err := os.Mkdir(j.Path+".flushed", 0o755); err != nil { // a directory where the offset file goes
		t.Fatal(err)
	}
	_, err := memory.Flush(context.Background(), j, func(context.Context, memory.Entry) error { return nil })
	if err == nil {
		t.Fatal("expected an error when the offset cannot be stored")
	}
}

func TestRecordJournalsOnlyWhenTheModeWritesToEngram(t *testing.T) {
	home, j := t.TempDir(), journalIn(t)
	testkit.Check(t, memory.Record(home, j, memory.Entry{Title: "a", Body: "b"}))
	testkit.Equal(t, "both journals", len(pending(t, j)), 1)

	testkit.Check(t, memory.SetOverride(home, memory.ClaudeOnly))
	testkit.Check(t, memory.Record(home, j, memory.Entry{Title: "c", Body: "d"}))
	testkit.Equal(t, "claude-only does not journal", len(pending(t, j)), 1)
}

func TestFlushPendingSavesThroughEngramAndSecondRunAddsNothing(t *testing.T) {
	log := testkit.StubEngram(t, `echo "$*" >> "$LOG"`)
	home, j := t.TempDir(), journalIn(t)
	fill(t, j, "one")

	n, err := memory.FlushPending(context.Background(), home, j)
	testkit.Check(t, err)
	testkit.Equal(t, "flushed", n, 1)
	n, _ = memory.FlushPending(context.Background(), home, j)
	testkit.Equal(t, "second run", n, 0)
	data, _ := os.ReadFile(log)
	testkit.Equal(t, "engram save calls", strings.Count(string(data), "save one"), 1)
}

func TestFlushPendingKeepsTheJournalWhenEngramIsMissingOrOff(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv(engram.BinaryEnv, "")
	home, j := t.TempDir(), journalIn(t)
	fill(t, j, "one")

	n, err := memory.FlushPending(context.Background(), home, j)
	testkit.Check(t, err)
	testkit.Equal(t, "missing engram flushes nothing", n, 0)

	testkit.StubEngram(t, "exit 0")
	testkit.Check(t, memory.SetOverride(home, memory.Off))
	n, err = memory.FlushPending(context.Background(), home, j)
	testkit.Check(t, err)
	testkit.Equal(t, "off flushes nothing", n, 0)
	testkit.Equal(t, "journal kept", len(pending(t, j)), 1)
}

func TestFlushPendingReportsEngramFailure(t *testing.T) {
	testkit.StubEngram(t, "exit 1")
	j := journalIn(t)
	fill(t, j, "one")
	if _, err := memory.FlushPending(context.Background(), t.TempDir(), j); err == nil {
		t.Fatal("a failing engram must be reported")
	}
}

func pending(t *testing.T, j memory.Journal) []memory.Entry {
	t.Helper()
	p, _, err := j.Pending()
	testkit.Check(t, err)
	return p
}
