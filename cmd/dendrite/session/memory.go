package session

import (
	"Dendrite/pkg/memory"
	"Dendrite/pkg/projectid"
	"context"
	"fmt"
	"io"
	"time"
)

// rememberCheckpoint journals the decisions and notes of a checkpoint and flushes the journal into Engram,
// so what the session settled is there for the next one. It is best-effort: a checkpoint never fails
// because memory could not be written.
func rememberCheckpoint(ctx context.Context, o SessionCheckpointOptions, ticket string, say io.Writer) {
	home, err := projectid.HomeDir()
	if err != nil {
		return
	}
	j := memory.DefaultJournal()
	add := func(typ, label, text string) {
		_ = memory.Record(home, j, memory.Entry{Type: typ, Ticket: ticket, Source: "session checkpoint", Title: label + ": " + memory.Clip(text, 100), Body: text})
	}
	for _, d := range o.Decisions {
		add("decision", "Decision", d)
	}
	for _, n := range o.Notes {
		add("note", "Note", n)
	}
	n, err := memory.FlushPending(ctx, home, j)
	switch {
	case err != nil:
		fmt.Fprintf(say, "[!] memory not flushed to Engram: %v\n", err)
	case n > 0:
		fmt.Fprintf(say, "[✓] %d memory item(s) flushed to Engram\n", n)
	}
}

// printMemoryMode tells a session when the weekly memory check switched the mode, and why.
func printMemoryMode(out io.Writer) {
	home, err := projectid.HomeDir()
	if err != nil {
		return
	}
	if notice := memory.ModeNotice(memory.LoadState(home), time.Now()); notice != "" {
		fmt.Fprintln(out, notice)
	}
}
