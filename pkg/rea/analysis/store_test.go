package analysis

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidIDKeepsPurgeInsideTheStore(t *testing.T) {
	for id, ok := range map[string]bool{
		"pe-1a2b3c4d": true, "a": true, "A.b_c-9": true, strings.Repeat("a", 64): true,
		"": false, ".": false, "..": false, "../x": false, "a/b": false, "-x": false, "a b": false, strings.Repeat("a", 65): false,
	} {
		if err := ValidID(id); (err == nil) != ok {
			t.Errorf("ValidID(%q) = %v, want ok=%t", id, err, ok)
		}
	}
}

func TestNewIDDependsOnTheBinaryAndTheMoment(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	a, b := NewID("/x/a.exe", now), NewID("/x/b.exe", now)
	if a == b || a != NewID("/x/a.exe", now) || !strings.HasPrefix(a, "pe-") || ValidID(a) != nil {
		t.Errorf("ids %s %s", a, b)
	}
	if a == NewID("/x/a.exe", now.Add(time.Nanosecond)) {
		t.Error("another moment, another id")
	}
}

func TestDefaultStoreLivesUnderTheDendriteHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	s, err := DefaultStore()
	if err != nil || s.Root != filepath.Join(home, "rea") {
		t.Errorf("%+v, %v", s, err)
	}
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := DefaultStore(); err == nil {
		t.Error("no home directory is an error")
	}
}

func TestStoreRefusesUnsafeIDs(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	if err := s.Create(State{ID: "../x"}); !errors.Is(err, ErrBadID) {
		t.Errorf("create with an unsafe id: %v", err)
	}
	if _, err := s.Load("../x"); !errors.Is(err, ErrBadID) {
		t.Errorf("load with an unsafe id: %v", err)
	}
	if err := s.Delete("../x"); !errors.Is(err, ErrBadID) {
		t.Errorf("delete with an unsafe id: %v", err)
	}
}

func TestStoreLoadReportsCheckpointsItCannotRead(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	if err := os.MkdirAll(s.Dir("corrupt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.statePath("corrupt"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("corrupt"); err == nil || !strings.Contains(err.Error(), "unreadable checkpoint") {
		t.Errorf("a corrupt checkpoint: %v", err)
	}
	if err := os.MkdirAll(s.statePath("dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("dir"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("a checkpoint that cannot be read is not 'not found': %v", err)
	}
}

func TestNewServiceUsesTheDendriteHomeAndTheSharedSidecars(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DENDRITE_HOME", home)
	s, err := NewService()
	if err != nil || s.Store.Root != filepath.Join(home, "rea") || s.Parser == nil || s.Index == nil {
		t.Fatalf("%+v, %v", s, err)
	}
	t.Setenv("DENDRITE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := NewService(); err == nil {
		t.Error("no home directory is an error")
	}
}
