package testkit

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileCreatesParentsAndAppliesTheMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a", "b", "f.txt")
	WriteFile(t, p, "hello", 0o600)
	info, err := os.Stat(p)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("stat = %v, %v", info, err)
	}
	if got, _ := os.ReadFile(p); string(got) != "hello" {
		t.Errorf("content = %q", got)
	}
}

func TestRegularFileExists(t *testing.T) {
	if info, err := os.Stat(RegularFile(t)); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("stat = %v, %v", info, err)
	}
}

func TestFreePortIsUsable(t *testing.T) {
	if p := FreePort(t); p <= 0 || p > 65535 {
		t.Fatalf("port = %d", p)
	}
}

func TestGitInitNamesTheFirstBranch(t *testing.T) {
	dir := t.TempDir()
	GitInit(t, dir, "trunk")
	if got := Git(t, dir, "symbolic-ref", "--short", "HEAD"); got != "trunk" {
		t.Fatalf("branch = %q", got)
	}
}

// recorder is a T whose Fatal stops the helper the way *testing.T does, and remembers the message.
type recorder struct {
	dir string
	msg string
}

type stopped struct{}

func (r *recorder) Helper()           {}
func (r *recorder) TempDir() string   { return r.dir }
func (r *recorder) Fatal(args ...any) { r.msg = fmt.Sprint(args...); panic(stopped{}) }
func (r *recorder) Fatalf(format string, args ...any) {
	r.msg = fmt.Sprintf(format, args...)
	panic(stopped{})
}

func (r *recorder) Errorf(format string, args ...any) { r.msg = fmt.Sprintf(format, args...) }

func TestEqualReportsOnlyAMismatch(t *testing.T) {
	r := &recorder{}
	Equal(r, "same", 3, 3)
	if r.msg != "" {
		t.Fatalf("equal values reported %q", r.msg)
	}
	Equal(r, "count", 3, 4)
	if r.msg != "count = 3, want 4" {
		t.Fatalf("mismatch reported %q", r.msg)
	}
}

// failsWith runs fn and returns what the helper reported before stopping ("" when it did not).
func failsWith(r *recorder, fn func()) (msg string) {
	defer func() {
		if p := recover(); p != nil {
			if _, ok := p.(stopped); !ok {
				panic(p)
			}
			msg = r.msg
		}
	}()
	fn()
	return ""
}

func TestHelpersStopTheTestWhenTheyCannotDoTheirJob(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &recorder{dir: t.TempDir()}
	if failsWith(r, func() { WriteFile(r, filepath.Join(file, "sub", "f"), "x", 0o644) }) == "" {
		t.Error("a parent that is a file cannot be created")
	}
	if failsWith(r, func() { WriteFile(r, t.TempDir(), "x", 0o644) }) == "" {
		t.Error("a path that is a directory cannot be written")
	}
	if failsWith(r, func() { Git(r, t.TempDir(), "no-such-subcommand") }) == "" {
		t.Error("a failing git command stops the test")
	}
}

func TestFreePortStopsTheTestWhenNothingCanListen(t *testing.T) {
	old := listen
	listen = func(string, string) (net.Listener, error) { return nil, errors.New("no network") }
	t.Cleanup(func() { listen = old })
	r := &recorder{dir: t.TempDir()}
	if failsWith(r, func() { FreePort(r) }) == "" {
		t.Error("expected the helper to stop the test")
	}
}

func TestCheckStopsTheTestOnlyOnAnError(t *testing.T) {
	Check(t, nil)
	r := &recorder{dir: t.TempDir()}
	if failsWith(r, func() { Check(r, errors.New("boom")) }) != "boom" {
		t.Error("Check must stop the test with the error")
	}
}

func TestBuildReturnsTheValueOrStopsTheTest(t *testing.T) {
	if got := Build(t, func() (int, error) { return 3, nil }); got != 3 {
		t.Fatalf("Build = %d", got)
	}
	r := &recorder{dir: t.TempDir()}
	if failsWith(r, func() { Build(r, func() (int, error) { return 0, errors.New("nope") }) }) != "nope" {
		t.Error("Build must stop the test with the error")
	}
}

func TestTableChecksEveryInput(t *testing.T) {
	r := &recorder{}
	Table(r, map[string]int{"a": 1, "bb": 2}, func(in string) int { return len(in) })
	if r.msg != "a = 1, want 1" && r.msg != "" {
		t.Fatalf("unexpected report %q", r.msg)
	}
	Table(r, map[string]int{"ccc": 1}, func(in string) int { return len(in) })
	if r.msg != "ccc = 3, want 1" {
		t.Fatalf("mismatch reported %q", r.msg)
	}
}
