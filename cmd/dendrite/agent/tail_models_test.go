package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/agentjobs"
)

func TestRunJobsTailShowsTheLastLinesOfTheLog(t *testing.T) {
	l := agentjobs.NewLedger(t.TempDir())
	id := "DEN-9-20990101T000000Z-aaaaaa"
	if err := os.MkdirAll(filepath.Dir(l.AgentLogPath(id)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.AgentLogPath(id), []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runJobsTail(l, id, 2, &out); err != nil || out.String() != "two\nthree\n" {
		t.Fatalf("tail = %q, %v", out.String(), err)
	}
	if err := runJobsTail(l, id, 0, &out); err == nil {
		t.Error("a zero line count was accepted")
	}
	out.Reset()
	if err := runJobsTail(l, "DEN-9-20990101T000000Z-bbbbbb", 3, &out); err != nil || !strings.Contains(out.String(), "No agent.log found") {
		t.Errorf("missing log: %q, %v", out.String(), err)
	}
	dirID := "DEN-9-20990101T000000Z-cccccc"
	if err := os.MkdirAll(l.AgentLogPath(dirID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runJobsTail(l, dirID, 3, &out); err == nil || !strings.Contains(err.Error(), "read agent.log") {
		t.Errorf("a log that cannot be read: err = %v", err)
	}
}

func TestRunJobsModelsReportsWhenAGYIsAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var out, errOut bytes.Buffer
	if err := runJobsModels(t.Context(), &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "[!]") || !strings.Contains(out.String(), "complexity 8 ->") {
		t.Errorf("stdout = %q, stderr = %q", out.String(), errOut.String())
	}
}
