package projectid

import (
	"Dendrite/pkg/fsutil"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FileName is the persistent project identifier file stored at <dir>/.dendrite/project.id.
const FileName = "project.id"

// Prefix defines the canonical prefix for all Dendrite project IDs.
const Prefix = "dpid_"

// ErrNoProjectID indicates that no .dendrite/project.id file exists in the directory.
var ErrNoProjectID = errors.New("projectid: no .dendrite/project.id")

var idRegex = regexp.MustCompile(`^dpid_[0-9a-f]{32}$`)

// Path returns the canonical path to <dir>/.dendrite/project.id; a linked worktree
// resolves to its main checkout's file (see StateRoot).
func Path(dir string) string {
	return filepath.Join(StateRoot(dir), ".dendrite", FileName)
}

// Valid reports whether s matches the ID format ^dpid_[0-9a-f]{32}$.
func Valid(s string) bool {
	return idRegex.MatchString(s)
}

// Read returns the trimmed ID from <dir>/.dendrite/project.id.
// It returns ErrNoProjectID (wrapped so errors.Is works) when absent,
// or an error if the content does not match ^dpid_[0-9a-f]{32}$.
func Read(dir string) (string, error) {
	p := Path(dir)
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrNoProjectID, p)
		}
		return "", err
	}

	id := strings.TrimSpace(string(data))
	if !Valid(id) {
		return "", fmt.Errorf("projectid: invalid project id %q in %s", id, p)
	}

	return id, nil
}

// Ensure returns the existing project ID or mints a new one:
// Prefix + first 32 hex chars of sha256(32 random bytes from crypto/rand || abs(dir) || time.Now().UnixNano()).
// It creates the .dendrite directory with permission 0o755 and writes "<id>\n" atomically
// (using a temporary file in the same directory and os.Rename, permission 0o644).
// It never rewrites an existing valid file.
func Ensure(dir string) (string, error) {
	id, err := Read(dir)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, ErrNoProjectID) {
		return "", err
	}

	// Abs only fails when the working directory is gone; the raw path still salts the hash.
	absDir, _ := filepath.Abs(dir)
	randBytes := make([]byte, 32)
	_, _ = rand.Read(randBytes) // crypto/rand.Read never returns an error (Go >= 1.24).

	h := sha256.New()
	h.Write(randBytes)
	h.Write([]byte(absDir))
	var nanoBuf [8]byte
	binary.BigEndian.PutUint64(nanoBuf[:], uint64(time.Now().UnixNano()))
	h.Write(nanoBuf[:])
	newID := Prefix + hex.EncodeToString(h.Sum(nil))[:32]

	if err := fsutil.WriteFileAtomic(Path(dir), []byte(newID+"\n")); err != nil {
		return "", fmt.Errorf("projectid: write %s: %w", Path(dir), err)
	}
	return newID, nil
}

// GitRemote returns `git -C dir remote get-url origin` trimmed,
// or "" if dir is not a git repository or has no origin remote.
func GitRemote(dir string) string {
	cmd := exec.Command("git", "-C", dir, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// NormalizeRemote canonicalises git remotes so ssh and https forms compare equal:
//
//	"git@github.com:A/b.git"        -> "github.com/a/b"
//	"https://github.com/A/b"        -> "github.com/a/b"
//	"ssh://git@host:22/a/b.git"     -> "host/a/b"
//
// It strips user@, scheme, port, trailing ".git" and "/", and lowercases the result.
// Passing "" returns "".
func NormalizeRemote(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}

	var host, path string
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			// Unparseable URLs still compare equal to themselves.
			return s
		}
		host, path = u.Hostname(), u.Path
	} else {
		// SCP-like syntax (git@github.com:org/repo.git) or a plain host/path.
		rest := s
		if at := strings.Index(rest, "@"); at != -1 && at < strings.IndexAny(rest+":", ":/") {
			rest = rest[at+1:]
		}
		if i := strings.IndexAny(rest, ":/"); i != -1 {
			host, path = rest[:i], rest[i+1:]
		} else {
			host = rest
		}
	}

	combined := host
	if path = strings.Trim(path, "/"); path != "" {
		combined += "/" + path
	}
	for strings.HasSuffix(combined, "/") || strings.HasSuffix(combined, ".git") {
		combined = strings.TrimSuffix(strings.TrimSuffix(combined, "/"), ".git")
	}
	return combined
}
