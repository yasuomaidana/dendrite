package synapse

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/clitest"
	"Dendrite/pkg/synapse"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type synapseCmdErrCase struct {
	args []string
	want string
}

func assertSynapseCommandErrors(t *testing.T, run func(args ...string) (string, error), cases []synapseCmdErrCase) {
	t.Helper()
	for _, tc := range cases {
		if _, err := run(tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

// clonedModule creates a bare origin with one Go module commit on develop and returns a
// clone of it plus the seed clone used to publish further commits.
func clonedModule(t *testing.T, root, name, goVersion string) (clone, seed string) {
	t.Helper()
	origin := filepath.Join(root, name+".git")
	seed = filepath.Join(root, "seed-"+name)
	clone = filepath.Join(root, name)
	clitest.GitRun(t, root, "init", "-q", "--bare", "-b", "develop", origin)
	clitest.GitRun(t, root, "clone", "-q", origin, seed)
	clitest.GitRun(t, seed, "checkout", "-q", "-b", "develop")
	if err := os.WriteFile(filepath.Join(seed, "go.mod"), []byte("module example.com/"+name+"\n\ngo "+goVersion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clitest.GitRun(t, seed, "add", "go.mod")
	clitest.GitRun(t, seed, "commit", "-q", "-m", "init")
	clitest.GitRun(t, seed, "push", "-q", "-u", "origin", "develop")
	clitest.GitRun(t, root, "clone", "-q", "-b", "develop", origin, clone)
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
	clitest.GitRun(t, hubSeed, "add", "-A")
	clitest.GitRun(t, hubSeed, "commit", "-q", "-m", "a")
	clitest.GitRun(t, hubSeed, "push", "-q", "origin", "develop")
	clitest.GitRun(t, hubSeed, "checkout", "-q", "-b", "feat/b")
	if err := os.WriteFile(filepath.Join(hubSeed, "b.go"), []byte("package hub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clitest.GitRun(t, hubSeed, "add", "-A")
	clitest.GitRun(t, hubSeed, "commit", "-q", "-m", "b")
	clitest.GitRun(t, hubSeed, "push", "-q", "origin", "feat/b")

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
	if m, _ := s.Member("hub"); m.LockedSHA != clitest.GitRun(t, hub, "rev-parse", "HEAD") || s.State != synapse.StateActive {
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

	assertSynapseCommandErrors(t, run, []synapseCmdErrCase{
		{[]string{"sync"}, "accepts 1 arg(s), received 0"},
		{[]string{"sync", "nope"}, "nope"},
		{[]string{"sync", "stack", "extra"}, "accepts 1 arg(s), received 2"},
		{[]string{"sync", "stack", "--merge"}, "needs an argument"},
		{[]string{"sync", "stack", "--merge=hub"}, "invalid --merge"},
		{[]string{"sync", "stack", "--merge", "ghost=x"}, "no member"},
		{[]string{"status", "stack", "--autostash"}, "unknown flag"},
	})
}

func TestPrintSyncReportVariants(t *testing.T) {
	var out bytes.Buffer
	PrintSynapseSyncReport(&out, &synapse.SyncReport{
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
	if cli.Short("") != "-" || cli.Short("abc") != "abc" {
		t.Fatal("short")
	}
}

// staleStack creates synapse "stack" over two in-sync checkouts and returns its store and the
// hub checkout. storeAs writes a state into the manifest, as an earlier status would have.
func staleStack(t *testing.T) (*synapse.Store, string) {
	t.Helper()
	for k, v := range map[string]string{
		"HOME": t.TempDir(), "DENDRITE_HOME": t.TempDir(), "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
	} {
		t.Setenv(k, v)
	}
	root := t.TempDir()
	hub, _ := clonedModule(t, root, "hub", "1.25")
	control, _ := clonedModule(t, root, "control", "1.25")
	runOK(t, "create", "stack", "--members", control+"=control:consumer,"+hub+"=hub:producer")
	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	storeAs(t, st, synapse.StateDrifted)
	return st, hub
}

func storeAs(t *testing.T, st *synapse.Store, state synapse.State) {
	t.Helper()
	s, err := st.Load("stack")
	if err != nil {
		t.Fatal(err)
	}
	s.State = state
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
}

func assertStored(t *testing.T, st *synapse.Store, want synapse.State) {
	t.Helper()
	s, err := st.Load("stack")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != want {
		t.Fatalf("stored state = %s, want %s", s.State, want)
	}
}

// assertOutput checks that out holds every want and none of the unwanted strings.
func assertOutput(t *testing.T, out string, want, unwanted []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output lacks %q:\n%s", w, out)
		}
	}
	for _, u := range unwanted {
		if strings.Contains(out, u) {
			t.Errorf("output holds %q:\n%s", u, out)
		}
	}
}

// list shows the state the members are in now, not the one an earlier status stored.
func TestSynapseCommand_ListRefreshesState(t *testing.T) {
	st, _ := staleStack(t)
	assertOutput(t, runOK(t, "list"), []string{"active"}, []string{"drifted"})
	assertStored(t, st, synapse.StateActive)

	storeAs(t, st, synapse.StateDrifted)
	var listed []struct{ State string }
	if err := json.Unmarshal([]byte(runOK(t, "list", "--json")), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].State != "active" {
		t.Fatalf("list --json = %+v", listed)
	}
}

// heal --auto stores the state the members are in now; a dry run changes nothing.
func TestSynapseCommand_HealAutoRefreshesState(t *testing.T) {
	st, _ := staleStack(t)
	runOK(t, "heal")
	assertStored(t, st, synapse.StateDrifted)
	runOK(t, "heal", "--auto")
	assertStored(t, st, synapse.StateActive)
}

// A checkout that is where the manifest says but lost its identity file is named as such,
// even when another clone of the same remote sits next to it, and --auto restores it.
func TestSynapseCommand_HealNamesMissingIdentity(t *testing.T) {
	st, hub := staleStack(t)
	if err := os.Remove(filepath.Join(hub, ".dendrite", "project.id")); err != nil {
		t.Fatal(err)
	}
	assertOutput(t, runOK(t, "heal"), []string{"identity missing", "--auto"}, []string{"->", "AMBIGUOUS"})
	assertOutput(t, runOK(t, "heal", "--auto"), []string{"identity restored"}, []string{"[!]"})
	assertOutput(t, runOK(t, "list"), []string{"active", "ok"}, []string{"unresolved"})
	assertStored(t, st, synapse.StateActive)
}
