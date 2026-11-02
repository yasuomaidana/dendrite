// Package testkit holds the small helpers that many test packages need: files, git repositories,
// free ports. Tests call these instead of each carrying its own copy.
package testkit

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// T is the part of *testing.T the helpers use.
type T interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Errorf(format string, args ...any)
	TempDir() string
}

// WriteFile writes content to path with the given mode, creating the directory above it.
func WriteFile(t T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// RegularFile creates a small file in a fresh temporary directory and returns its path.
func RegularFile(t T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "file")
	WriteFile(t, p, "x", 0o644)
	return p
}

// listen is net.Listen; the tests replace it to reach the failure.
var listen = net.Listen

// FreePort returns a TCP port on 127.0.0.1 that nothing listens on right now.
func FreePort(t T) int {
	t.Helper()
	ln, err := listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// Git runs git in dir with a fixed author and committer and returns its trimmed combined output.
func Git(t T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// GitInit makes dir a git repository whose first branch is called branch.
func GitInit(t T, dir, branch string) {
	t.Helper()
	Git(t, dir, "init", "-q")
	Git(t, dir, "symbolic-ref", "HEAD", "refs/heads/"+branch)
}

// Check stops the test when err is not nil.
func Check(t T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Build returns what build makes, stopping the test when it fails.
func Build[V any](t T, build func() (V, error)) V {
	t.Helper()
	v, err := build()
	Check(t, err)
	return v
}

// Equal reports a mismatch between got and want under the given label without stopping the test.
func Equal[V comparable](t T, label string, got, want V) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

// Table checks fn against every input of cases and reports each mismatch without stopping the test.
func Table[V comparable](t T, cases map[string]V, fn func(input string) V) {
	t.Helper()
	for input, want := range cases {
		Equal(t, input, fn(input), want)
	}
}
