// Package memory is Dendrite's agent-memory write path: events worth remembering are appended to a local
// journal the moment they happen, and flushed into Engram when a session ends. The journal is the
// durable part (an fsynced JSON line per entry); Engram stays the store the next session reads from.
package memory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/statedir"
)

// Entry is one thing worth remembering. Type is Engram's observation type (decision, plan, note, outcome).
type Entry struct {
	ID      string    `json:"id"`
	Time    time.Time `json:"time"`
	Project string    `json:"project,omitempty"`
	Ticket  string    `json:"ticket,omitempty"`
	Type    string    `json:"type,omitempty"`
	Title   string    `json:"title"`
	Body    string    `json:"body"`
	Source  string    `json:"source,omitempty"`
}

// EntryID is the stable identity of an entry: the same memory written twice gets the same ID, whatever
// the time, so a replay never duplicates it.
func EntryID(e Entry) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{e.Project, e.Type, e.Title, e.Body}, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// flushedSuffix names the file beside the journal that holds the byte offset already flushed to Engram.
const flushedSuffix = ".flushed"

// Journal is an append-only JSONL file of entries plus a flushed offset.
type Journal struct{ Path string }

// DefaultJournal is the journal of the main checkout (every working copy of a repository shares it).
func DefaultJournal() Journal {
	return Journal{Path: statedir.Resolve(".dendrite/session/memory-journal.jsonl")}
}

// Append writes e as one fsynced line and returns it with its ID and time filled. A torn tail left by a
// killed writer is cut first, so the new line never glues onto half a line.
func (j Journal) Append(e Entry) (Entry, error) {
	if strings.TrimSpace(e.Title) == "" || strings.TrimSpace(e.Body) == "" {
		return Entry{}, errors.New("memory: an entry needs a title and a body")
	}
	e.ID = EntryID(e)
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	if err := dropTornTail(j.Path); err != nil {
		return Entry{}, err
	}
	line, _ := json.Marshal(e)
	return e, fsutil.AppendLine(j.Path, line)
}

// dropTornTail truncates path to its last complete line; a missing file is fine.
func dropTornTail(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) == 0 || data[len(data)-1] == '\n' {
		return nil
	}
	return os.Truncate(path, int64(bytes.LastIndexByte(data, '\n')+1))
}

// Pending returns the entries not yet flushed and the offset just after the last complete line, which is
// what MarkFlushed wants once they are safe in Engram. A torn last line and garbled lines are skipped.
func (j Journal) Pending() ([]Entry, int64, error) {
	data, err := os.ReadFile(j.Path)
	if os.IsNotExist(err) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	pos := j.flushedOffset(int64(len(data)))
	var out []Entry
	for {
		i := bytes.IndexByte(data[pos:], '\n')
		if i < 0 {
			return out, pos, nil
		}
		line := data[pos : pos+int64(i)]
		pos += int64(i) + 1
		var e Entry
		if json.Unmarshal(line, &e) == nil && e.Title != "" {
			out = append(out, e)
		}
	}
}

// flushedOffset is the recorded offset, or 0 (replay everything) when it is missing, unreadable or beyond
// the journal: Engram dedupes, so a replay is harmless and a lost memory is not.
func (j Journal) flushedOffset(size int64) int64 {
	raw, err := os.ReadFile(j.Path + flushedSuffix)
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || n < 0 || n > size {
		return 0
	}
	return n
}

// MarkFlushed records that everything before offset reached Engram.
func (j Journal) MarkFlushed(offset int64) error {
	return fsutil.WriteFileAtomic(j.Path+flushedSuffix, []byte(strconv.FormatInt(offset, 10)))
}
