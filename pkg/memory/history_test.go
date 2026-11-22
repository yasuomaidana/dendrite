package memory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/memory"
	"Dendrite/pkg/testkit"
)

func report(candidate memory.Mode, healthy bool) memory.Report {
	return memory.Report{Time: now, Candidate: candidate, EngramHealthy: healthy, Note: "engram is unhealthy: x"}
}

func TestDecideChangesTheModeOnlyAfterTwoAgreeingChecks(t *testing.T) {
	first := report(memory.EngramOnly, true)
	s := memory.Decide(memory.State{}, nil, first, now)
	testkit.Equal(t, "one check moves nothing", s.Computed, memory.Mode(""))

	s = memory.Decide(s, &first, report(memory.ClaudeOnly, true), now)
	testkit.Equal(t, "a disagreeing check moves nothing", s.Computed, memory.Mode(""))

	agreeing := report(memory.ClaudeOnly, true)
	s = memory.Decide(s, &agreeing, report(memory.ClaudeOnly, true), now.Add(time.Hour))
	testkit.Equal(t, "two agreeing checks switch", s.Computed, memory.ClaudeOnly)
	testkit.Equal(t, "since", s.Since, now.Add(time.Hour))
	if !strings.Contains(s.Reason, "engram") {
		t.Errorf("reason = %q", s.Reason)
	}

	again := memory.Decide(s, &agreeing, report(memory.ClaudeOnly, true), now.Add(48*time.Hour))
	testkit.Equal(t, "an unchanged mode keeps its date", again.Since, s.Since)
}

func TestAnUnhealthyEngramSwitchesItOffAtOnce(t *testing.T) {
	s := memory.Decide(memory.State{Computed: memory.EngramOnly}, nil, report(memory.ClaudeOnly, false), now)
	testkit.Equal(t, "computed", s.Computed, memory.ClaudeOnly)
	testkit.Equal(t, "reason", s.Reason, "engram is unhealthy: x")
}

func TestAnUnhealthyLastCheckDoesNotCountTowardsASwitch(t *testing.T) {
	last := report(memory.Both, false)
	s := memory.Decide(memory.State{Computed: memory.ClaudeOnly}, &last, report(memory.Both, true), now)
	testkit.Equal(t, "still claude", s.Computed, memory.ClaudeOnly)
}

func TestChecksAreRecordedAndTheLastOneIsRead(t *testing.T) {
	home := t.TempDir()
	if _, ok := memory.LastCheck(home); ok {
		t.Fatal("no checks yet")
	}
	testkit.Check(t, memory.AppendCheck(home, report(memory.Both, true)))
	testkit.Check(t, memory.AppendCheck(home, report(memory.EngramOnly, true)))
	f, _ := os.OpenFile(memory.ChecksPath(home), os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("garbage\n")
	_ = f.Close()

	last, ok := memory.LastCheck(home)
	if !ok || last.Candidate != memory.EngramOnly {
		t.Fatalf("last = %+v, %v (a garbled line is skipped)", last, ok)
	}
}

func TestAJournalOfOnlyGarbageHasNoLastCheck(t *testing.T) {
	home := t.TempDir()
	testkit.Check(t, os.MkdirAll(filepath.Dir(memory.ChecksPath(home)), 0o755))
	testkit.Check(t, os.WriteFile(memory.ChecksPath(home), []byte("garbage\n{}\n"), 0o644))
	if _, ok := memory.LastCheck(home); ok {
		t.Fatal("garbage and reports without a time are not checks")
	}
}

func TestModeNoticeSpeaksOnlyAboutARecentComputedSwitch(t *testing.T) {
	s := memory.State{Computed: memory.ClaudeOnly, Reason: "engram unhealthy", Since: now.Add(-24 * time.Hour)}
	if got := memory.ModeNotice(s, now); !strings.Contains(got, "claude since 2026-11-21") || !strings.Contains(got, "engram unhealthy") {
		t.Fatalf("notice = %q", got)
	}
	for name, quiet := range map[string]memory.State{
		"override wins": {Override: memory.Off, Computed: memory.ClaudeOnly, Since: now},
		"no computed":   {},
		"no date":       {Computed: memory.ClaudeOnly},
		"old":           {Computed: memory.ClaudeOnly, Since: now.Add(-30 * 24 * time.Hour)},
	} {
		testkit.Equal(t, name, memory.ModeNotice(quiet, now), "")
	}
}
