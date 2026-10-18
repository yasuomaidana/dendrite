package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/synapse"
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// clonedModule creates a bare origin with one Go module commit on develop and returns a
// clone of it plus the seed clone used to publish further commits.
func clonedModule(t *testing.T, root, name, goVersion string) (clone, seed string) {
	t.Helper()
	origin := filepath.Join(root, name+".git")
	seed = filepath.Join(root, "seed-"+name)
	clone = filepath.Join(root, name)
	gitRun(t, root, "init", "-q", "--bare", "-b", "develop", origin)
	gitRun(t, root, "clone", "-q", origin, seed)
	gitRun(t, seed, "checkout", "-q", "-b", "develop")
	if err := os.WriteFile(filepath.Join(seed, "go.mod"), []byte("module example.com/"+name+"\n\ngo "+goVersion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, seed, "add", "go.mod")
	gitRun(t, seed, "commit", "-q", "-m", "init")
	gitRun(t, seed, "push", "-q", "-u", "origin", "develop")
	gitRun(t, root, "clone", "-q", "-b", "develop", origin, clone)
	return clone, seed
}

func TestSynapseCommand_StatusAndSync(t *testing.T) {
	for k, v := range map[string]string{
		"DENDRITE_HOME": t.TempDir(), "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
	} {
		t.Setenv(k, v)
	}
	root := t.TempDir()
	hub, hubSeed := clonedModule(t, root, "hub", "1.25")
	control, _ := clonedModule(t, root, "control", "1.25")
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := runSynapseCommand(context.Background(), args, &out)
		return out.String(), err
	}
	if _, err := run("create", "stack", "--members", control+"=control:consumer,"+hub+"=hub:producer", "--sync", "go.work"); err != nil {
		t.Fatal(err)
	}

	// Publish a commit on hub's develop and a feature branch.
	for _, f := range []string{"a.go"} {
		if err := os.WriteFile(filepath.Join(hubSeed, f), []byte("package hub\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, hubSeed, "add", "-A")
	gitRun(t, hubSeed, "commit", "-q", "-m", "a")
	gitRun(t, hubSeed, "push", "-q", "origin", "develop")
	gitRun(t, hubSeed, "checkout", "-q", "-b", "feat/b")
	if err := os.WriteFile(filepath.Join(hubSeed, "b.go"), []byte("package hub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, hubSeed, "add", "-A")
	gitRun(t, hubSeed, "commit", "-q", "-m", "b")
	gitRun(t, hubSeed, "push", "-q", "origin", "feat/b")

	out, err := run("status", "stack")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Synapse stack: drifted", "1 commit(s) behind origin/develop", "origin/feat/b (1 commit(s))", "--merge hub=feat/b"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(control, "go.work")); !os.IsNotExist(err) {
		t.Fatal("status must not write go.work")
	}

	out, err = run("sync", "stack", "--merge", "hub=feat/b")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Synapse stack: active", "merged origin/feat/b", "[written] go.work", "(local: hub)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("sync missing %q:\n%s", want, out)
		}
	}
	st, _ := synapse.DefaultStore()
	s, _ := st.Load("stack")
	if m, _ := s.Member("hub"); m.LockedSHA != gitRun(t, hub, "rev-parse", "HEAD") || s.State != synapse.StateActive {
		t.Fatalf("persisted manifest = %+v", s)
	}

	out, err = run("sync", "stack", "--json", "--no-fetch", "--no-link", "--rebase", "--autostash")
	if err != nil {
		t.Fatal(err)
	}
	var rep synapse.SyncReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil || rep.Synapse != "stack" || len(rep.Links) != 0 {
		t.Fatalf("json report = %+v, %v\n%s", rep, err, out)
	}
	if out, err = run("status", "stack", "--no-fetch"); err != nil || !strings.Contains(out, "Synapse stack: active") {
		t.Fatalf("status after sync = %q, %v", out, err)
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"sync"}, "synapse id is required"},
		{[]string{"sync", "nope"}, "nope"},
		{[]string{"sync", "stack", "extra"}, "unexpected argument"},
		{[]string{"sync", "stack", "--merge"}, "requires"},
		{[]string{"sync", "stack", "--merge=hub"}, "invalid --merge"},
		{[]string{"sync", "stack", "--merge", "ghost=x"}, "no member"},
		{[]string{"status", "stack", "--autostash"}, "unknown flag"},
	} {
		if _, err := run(tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestPrintSyncReportVariants(t *testing.T) {
	var out bytes.Buffer
	printSyncReport(&out, &synapse.SyncReport{
		Synapse: "s", State: synapse.StateBroken,
		Members: []synapse.MemberSync{
			{Alias: "gone", Source: synapse.SourceLocal, Missing: true, Error: "missing"},
			{Alias: "far", Source: synapse.SourceRemote, Ref: "v1", LockedSHA: "abc"},
		},
		Links: []synapse.LinkResult{{Action: synapse.LinkSkipped, Strategy: "pnpm-workspace", Reason: "not implemented"}},
	})
	for _, want := range []string{"error: missing", "far     remote  v1        abc", "[skipped] pnpm-workspace - not implemented"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q:\n%s", want, out.String())
		}
	}
	if short("") != "-" || short("abc") != "abc" {
		t.Fatal("short")
	}
}
