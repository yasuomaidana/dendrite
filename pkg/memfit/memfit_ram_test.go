package memfit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// withSystem makes TotalRAMBytes read from the given operating system and readers.
func withSystem(t *testing.T, os string, sysctl, meminfo func() ([]byte, error)) {
	t.Helper()
	oldOS, oldSysctl, oldMeminfo := goos, readSysctl, readMeminfo
	t.Cleanup(func() { goos, readSysctl, readMeminfo = oldOS, oldSysctl, oldMeminfo })
	goos, readSysctl, readMeminfo = os, sysctl, meminfo
}

func bytesOf(s string) func() ([]byte, error) {
	return func() ([]byte, error) { return []byte(s), nil }
}
func failing() ([]byte, error) { return nil, errors.New("unreadable") }

func TestTotalRAMBytesPerOperatingSystem(t *testing.T) {
	cases := []struct {
		name    string
		goos    string
		sysctl  func() ([]byte, error)
		meminfo func() ([]byte, error)
		want    uint64
	}{
		{"darwin reads hw.memsize", "darwin", bytesOf("17179869184\n"), failing, 17179869184},
		{"darwin without sysctl", "darwin", failing, failing, 0},
		{"linux reads MemTotal in kB", "linux", failing, bytesOf("MemFree: 5 kB\nMemTotal:       16384 kB\n"), 16384 * 1024},
		{"linux without /proc/meminfo", "linux", failing, failing, 0},
		{"linux without a MemTotal line", "linux", failing, bytesOf("MemFree: 5 kB\nMemTotal:\n"), 0},
		{"other systems are unknown", "plan9", failing, failing, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			withSystem(t, c.goos, c.sysctl, c.meminfo)
			if got := TotalRAMBytes(); got != c.want {
				t.Fatalf("TotalRAMBytes = %d, want %d", got, c.want)
			}
		})
	}
}

func TestBudgetIsAConservativeFloorWhenRAMIsUnknown(t *testing.T) {
	withSystem(t, "plan9", failing, failing)
	t.Setenv(EnvBudgetGB, "")
	if got := BudgetBytes(); got != 4<<30 {
		t.Fatalf("budget = %d, want 4 GB", got)
	}
}

func TestAssessFallsBackToTheDefaultEndpoints(t *testing.T) {
	r := Assess(context.Background(), "", "")
	if r.BudgetBytes == 0 {
		t.Fatalf("report has no budget: %+v", r)
	}
}

func TestOxiServedNeedsAModelAndAnswer(t *testing.T) {
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer empty.Close()
	if _, _, err := oxiServed(context.Background(), http.DefaultClient, empty.URL); err == nil {
		t.Fatal("a server with no model must be an error")
	}
	if _, _, err := oxiServed(context.Background(), http.DefaultClient, "http://127.0.0.1:1"); err == nil {
		t.Fatal("an unreachable server must be an error")
	}
}

func TestGGUFSizeSkipsFilesItCannotStat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".oxillama", "models", "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "missing.gguf"), filepath.Join(dir, "model.gguf")); err != nil {
		t.Fatal(err)
	}
	if got := ggufSize("broken"); got != 0 {
		t.Fatalf("size = %d, want 0 for a dangling link", got)
	}
}

func TestResolveOllamaKeepsTheRequestWhenNothingFits(t *testing.T) {
	t.Setenv(EnvBudgetGB, "1")
	ollama, _ := fakeEngines(t, "")
	if got := ResolveOllama(context.Background(), ollama, "big:27b"); got != "big:27b" {
		t.Fatalf("resolved = %q, want the request back", got)
	}
}

func TestProcMeminfoIsReadableExactlyWhereProcExists(t *testing.T) {
	_, statErr := os.Stat("/proc/meminfo")
	data, err := readProcMeminfo()
	if (statErr == nil) != (err == nil) {
		t.Fatalf("stat error %v but read error %v", statErr, err)
	}
	if err == nil && len(data) == 0 {
		t.Fatal("/proc/meminfo is empty")
	}
}
