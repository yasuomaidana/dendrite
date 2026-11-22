package memory

import (
	"context"

	"Dendrite/pkg/engram"
)

// Saver stores one entry in a memory store.
type Saver func(ctx context.Context, e Entry) error

// Flush saves every pending entry through save and, only when all of them were saved, marks the journal
// flushed. A failure keeps the journal pending so the next flush retries; Engram dedupes, so the entries
// that did get through are not stored twice. It returns how many entries were saved.
func Flush(ctx context.Context, j Journal, save Saver) (int, error) {
	pending, end, err := j.Pending()
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	for i, e := range pending {
		if err := save(ctx, e); err != nil {
			return i, err
		}
	}
	return len(pending), j.MarkFlushed(end)
}

// Record journals e when the mode in force writes to Engram; otherwise there is nothing to flush later and
// it does nothing. Writers call it best-effort: a memory that cannot be journaled must not break the work.
func Record(home string, j Journal, e Entry) error {
	if !LoadState(home).Resolve().Engram() {
		return nil
	}
	_, err := j.Append(e)
	return err
}

// FlushPending is the session-end step: with Engram on and installed, save the pending entries into it.
// Without Engram (missing, or switched off by the mode) it keeps the journal and reports nothing wrong.
func FlushPending(ctx context.Context, home string, j Journal) (int, error) {
	if !LoadState(home).Resolve().Engram() {
		return 0, nil
	}
	bin, err := engram.Locate()
	if err != nil {
		return 0, nil
	}
	return Flush(ctx, j, func(ctx context.Context, e Entry) error {
		return engram.Save(ctx, bin, e.Project, e.Type, e.Title, e.Body)
	})
}
