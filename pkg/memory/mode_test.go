package memory_test

import (
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/memory"
	"Dendrite/pkg/testkit"
)

func TestParseMode(t *testing.T) {
	for in, want := range map[string]memory.Mode{
		"both": memory.Both, "engram": memory.EngramOnly, "claude": memory.ClaudeOnly, "off": memory.Off, "auto": "", "": "",
	} {
		got, err := memory.ParseMode(in)
		testkit.Check(t, err)
		testkit.Equal(t, "ParseMode("+in+")", got, want)
	}
	if _, err := memory.ParseMode("loud"); err == nil {
		t.Fatal("an unknown mode must be refused")
	}
}

func TestModeSwitches(t *testing.T) {
	for _, c := range []struct {
		mode           memory.Mode
		engram, claude bool
	}{
		{memory.Both, true, true}, {memory.EngramOnly, true, false}, {memory.ClaudeOnly, false, true}, {memory.Off, false, false},
	} {
		testkit.Equal(t, string(c.mode)+" engram", c.mode.Engram(), c.engram)
		testkit.Equal(t, string(c.mode)+" claude", c.mode.Claude(), c.claude)
	}
}

func TestResolveOrderIsOverrideThenComputedThenDefault(t *testing.T) {
	testkit.Equal(t, "default", memory.State{}.Resolve(), memory.Both)
	testkit.Equal(t, "computed", memory.State{Computed: memory.ClaudeOnly}.Resolve(), memory.ClaudeOnly)
	testkit.Equal(t, "override", memory.State{Override: memory.Off, Computed: memory.ClaudeOnly}.Resolve(), memory.Off)
}

func TestStateRoundTripAndSetOverride(t *testing.T) {
	home := t.TempDir()
	testkit.Equal(t, "missing state", memory.LoadState(home).Resolve(), memory.Both)

	testkit.Check(t, memory.SaveState(home, memory.State{Computed: memory.EngramOnly, Reason: "engram recalled more"}))
	testkit.Equal(t, "loaded", memory.LoadState(home).Resolve(), memory.EngramOnly)

	testkit.Check(t, memory.SetOverride(home, memory.Off))
	got := memory.LoadState(home)
	testkit.Equal(t, "override wins", got.Resolve(), memory.Off)
	testkit.Equal(t, "computed kept", got.Computed, memory.EngramOnly)

	testkit.Check(t, memory.SetOverride(home, ""))
	testkit.Equal(t, "auto clears the override", memory.LoadState(home).Resolve(), memory.EngramOnly)
}

func TestACorruptStateFileIsTreatedAsEmpty(t *testing.T) {
	home := t.TempDir()
	path := memory.StatePath(home)
	testkit.Check(t, os.MkdirAll(filepath.Dir(path), 0o755))
	testkit.Check(t, os.WriteFile(path, []byte("{nope"), 0o644))
	testkit.Equal(t, "corrupt", memory.LoadState(home).Resolve(), memory.Both)
}

func TestSaveStateFailsOnAnUnwritableHome(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	testkit.Check(t, os.WriteFile(file, nil, 0o644))
	if err := memory.SaveState(file, memory.State{}); err == nil {
		t.Fatal("expected an error")
	}
	if err := memory.SetOverride(file, memory.Off); err == nil {
		t.Fatal("SetOverride must report the failure")
	}
}
