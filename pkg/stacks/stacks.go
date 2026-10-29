// Package stacks answers one question: where do the stack's files live? The compose files, the
// generated litellm_config.yaml and what they mount sit in $DENDRITE_HOME/stacks, next to the
// credentials in $DENDRITE_HOME/.env, so the tool is configured once per machine and works from any
// directory. DENDRITE_STACKS_DIR overrides it (tests, a second stack).
package stacks

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
)

// EnvDir overrides the stacks directory.
const EnvDir = "DENDRITE_STACKS_DIR"

// Dir is the stacks directory: DENDRITE_STACKS_DIR, else $DENDRITE_HOME/stacks. When the home cannot
// be resolved it falls back to ./stacks, which is where the files lived before they moved.
func Dir() string {
	if d := strings.TrimSpace(os.Getenv(EnvDir)); d != "" {
		return d
	}
	// A test binary that sets neither this directory nor a home must never write into the real
	// ~/.dendrite: it gets a throwaway directory instead.
	if testing.Testing() && os.Getenv("DENDRITE_HOME") == "" {
		return filepath.Join(os.TempDir(), fmt.Sprintf("dendrite-test-stacks-%d", os.Getpid()))
	}
	home, err := projectid.HomeDir()
	if err != nil {
		return "stacks"
	}
	return filepath.Join(home, "stacks")
}

// Path is the location of a file in the stacks directory.
func Path(name string) string { return filepath.Join(Dir(), name) }

// skip lists what never moves: user credentials and templates stay where they are, and editor or
// VCS noise is not part of the stack.
func skip(rel string, d fs.DirEntry) bool {
	base := d.Name()
	switch {
	case base == ".DS_Store" || base == "__pycache__" || strings.HasSuffix(base, ".pyc"):
		return true
	case !d.IsDir() && (base == ".env" || strings.HasPrefix(base, ".env.")):
		return true // credentials live in $DENDRITE_HOME/.env; the template stays in the repository
	}
	return false
}

// Report says what Migrate did. It lists paths only.
type Report struct {
	Copied      []string // new files written to the destination
	Kept        []string // files already present at the destination, left untouched
	Source      string
	Destination string
}

// Migrate copies the stack files of a checkout (src, normally <repo>/stacks) into dst, creating it.
// A file that already exists at the destination is never overwritten, so re-running is safe and a
// hand-maintained destination always wins. With dryRun nothing is written.
func Migrate(src, dst string, dryRun bool) (*Report, error) {
	info, err := os.Stat(src)
	if err != nil {
		return nil, fmt.Errorf("stack source: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("stack source %s is not a directory", src)
	}
	if abs(src) == abs(dst) {
		return &Report{Source: src, Destination: dst}, nil
	}
	rep := &Report{Source: src, Destination: dst}
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		if skip(rel, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if dryRun {
				return nil
			}
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil // symlinks and devices are not part of a stack
		}
		if _, err := os.Stat(target); err == nil {
			rep.Kept = append(rep.Kept, rel)
			return nil
		}
		rep.Copied = append(rep.Copied, rel)
		if dryRun {
			return nil
		}
		return copyFile(path, target)
	})
	sort.Strings(rep.Copied)
	sort.Strings(rep.Kept)
	return rep, err
}

func abs(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// CheckoutStacks returns <repo>/stacks when a checkout rooted at root has one with a control compose
// file, "" otherwise. It is the source Migrate copies from.
func CheckoutStacks(root string) string {
	dir := filepath.Join(root, "stacks")
	if _, err := os.Stat(filepath.Join(dir, "compose.control.yaml")); err != nil {
		return ""
	}
	return dir
}
