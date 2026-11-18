package immune

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"Dendrite/pkg/fsutil"
)

// LedgerFile is where the scan ledger lives inside the local state directory (.dendrite/).
const LedgerFile = ".dendrite/immune/scans.jsonl"

// Entry is one line of the scan ledger: what was scanned, with which scanner, and the verdict.
type Entry struct {
	Time           time.Time `json:"time"`
	Path           string    `json:"path"`
	Skill          string    `json:"skill"`
	Digest         string    `json:"digest"`
	Score          int       `json:"score"`
	Verdict        Verdict   `json:"verdict"`
	ScannerVersion string    `json:"scanner_version"`
	LLM            bool      `json:"llm,omitempty"`
}

// same reports whether o records the same scan result as e, whenever each was taken.
func (e Entry) same(o Entry) bool {
	e.Time, o.Time = time.Time{}, time.Time{}
	return e == o
}

// Ledger is the append-only JSON-lines record of skill scans. Every change is made under a file
// lock, so sessions scanning at the same time neither lose nor interleave lines.
type Ledger struct{ Path string }

// Record appends e unless the latest entry for the same path already says the same thing, so
// scanning unchanged content again leaves the ledger as it was. It reports whether a line was added.
// The file is replaced atomically, so a reader never sees half a line.
func (l Ledger) Record(e Entry) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return false, err
	}
	release, err := fsutil.Lock(l.Path + ".lock")
	if err != nil {
		return false, err
	}
	defer release()

	data, entries, err := l.read()
	if err != nil {
		return false, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Path == e.Path {
			if entries[i].same(e) {
				return false, nil
			}
			break
		}
	}
	line, _ := json.Marshal(e) // an Entry holds nothing that cannot be encoded
	if err := fsutil.WriteFileAtomic(l.Path, append(append(data, line...), '\n')); err != nil {
		return false, err
	}
	return true, nil
}

// Entries reads the ledger, oldest first; a ledger that does not exist yet is empty. A line that
// is not an entry is an error: a record that cannot be read must not pass for a clean one.
func (l Ledger) Entries() ([]Entry, error) {
	_, entries, err := l.read()
	return entries, err
}

// read returns the ledger's bytes and the entries they hold.
func (l Ledger) read() ([]byte, []Entry, error) {
	data, _, err := fsutil.ReadIfPresent(l.Path)
	if err != nil {
		return nil, nil, err
	}
	var out []Entry
	for n, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, nil, fmt.Errorf("%s line %d: %w", l.Path, n+1, err)
		}
		out = append(out, e)
	}
	return data, out, nil
}

// Find returns the latest entry for content with this digest, wherever it was scanned.
func (l Ledger) Find(digest string) (Entry, bool, error) {
	entries, err := l.Entries()
	if err != nil {
		return Entry{}, false, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Digest == digest {
			return entries[i], true, nil
		}
	}
	return Entry{}, false, nil
}

// Digest is the SHA-256 of what a scan of path reads: the file, or every regular file below the
// directory with its relative name, in a fixed order. Git's own directory is not part of a skill.
func Digest(path string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(path, p)
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(data))
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
