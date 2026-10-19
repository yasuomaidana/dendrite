package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/synapsesync"
)


// syncTestRepo creates a repo on "feature" with one pushed commit, a bare origin and a project.id.
func syncTestRepo(t *testing.T, name string) (dir, bare string) {
	t.Helper()
	root := t.TempDir()
	dir, bare = filepath.Join(root, name), filepath.Join(root, name+".git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitT(t, root, "init", "-q", "--bare", "-b", "feature", bare)
	runGitT(t, dir, "init", "-q", "-b", "feature")
	runGitT(t, dir, "config", "user.email", "t@x")
	runGitT(t, dir, "config", "user.name", "t")
	runGitT(t, dir, "remote", "add", "origin", bare)
	if _, err := projectid.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	syncTestFile(t, dir, "README.md", true)
	runGitT(t, dir, "push", "-q", "-u", "origin", "feature")
	return dir, bare
}

func syncTestFile(t *testing.T, dir, name string, commit bool) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	if commit {
		runGitT(t, dir, "add", "-A")
		runGitT(t, dir, "commit", "-q", "-m", "add "+name)
	}
}

func saveTestSynapse(t *testing.T, id string, dirs map[string]string) {
	t.Helper()
	st, err := synapse.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	s := &synapse.Synapse{ID: id, Name: id, State: synapse.StateActive}
	for _, alias := range []string{"den", "git", "rag"} {
		dir, ok := dirs[alias]
		if !ok {
			continue
		}
		pid, err := projectid.Read(dir)
		if err != nil {
			t.Fatal(err)
		}
		s.Members = append(s.Members, synapse.Member{ProjectID: pid, Alias: alias, Role: synapse.RolePeer, PathHint: dir})
	}
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
}

func chdirT(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func TestParseSyncArgs(t *testing.T) {
	t.Setenv("PLANE_PROJECT_ID", "env-proj")
	t.Setenv("DENDRITE_SYNC_INTERVAL", "")
	opts, cli, ok, err := parseSyncArgs([]string{"--synapse", "mesh", "--plane=DGIT", "--git-only", "--force-plane", "--no-fetch", "--dry-run", "--json", "--watch", "--interval", "30s"})
	if err != nil || !ok || !cli.asJSON || !cli.watch || cli.interval != 30*time.Second {
		t.Fatalf("ok=%v cli=%+v err=%v", ok, cli, err)
	}
	if _, cli, _, _ = parseSyncArgs([]string{"--interval=2m"}); cli.interval != 2*time.Minute {
		t.Errorf("--interval=2m parsed as %s", cli.interval)
	}
	if _, cli, _, _ = parseSyncArgs(nil); cli.interval != synapsesync.DefaultInterval {
		t.Errorf("default interval %s", cli.interval)
	}
	if opts.SynapseID != "mesh" || opts.DefaultPlaneProject != "DGIT" || !opts.GitOnly || !opts.ForcePlane || opts.Fetch || !opts.DryRun {
		t.Fatalf("unexpected opts: %+v", opts)
	}
	opts, _, _, _ = parseSyncArgs([]string{"--synapse=x", "--plane", "DEN", "--all", "--plane-only"})
	if opts.SynapseID != "x" || opts.DefaultPlaneProject != "DEN" || !opts.All || !opts.PlaneOnly || !opts.Fetch {
		t.Fatalf("unexpected opts: %+v", opts)
	}
	if opts, _, _, _ = parseSyncArgs(nil); opts.DefaultPlaneProject != "env-proj" {
		t.Errorf("expected PLANE_PROJECT_ID default, got %q", opts.DefaultPlaneProject)
	}
	if _, _, ok, _ := parseSyncArgs([]string{"-h"}); ok {
		t.Error("expected usage request")
	}
	t.Setenv("DENDRITE_SYNC_INTERVAL", "0")
	for _, bad := range [][]string{{"--synapse"}, {"--plane"}, {"--bogus"}, {"--git-only", "--plane-only"}, {"--interval"}, {"--interval", "soon"}, {"--watch"}} {
		if _, _, _, err := parseSyncArgs(bad); err == nil {
			t.Errorf("expected error for %v", bad)
		}
	}
}

func TestRunSyncCommand(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv("DENDRITE_COMMIT_PUSH", "")
	t.Setenv("PLANE_BASE_URL", "http://127.0.0.1:1")
	ctx := context.Background()

	den, denBare := syncTestRepo(t, "den")
	gitDir, gitBare := syncTestRepo(t, "git")
	saveTestSynapse(t, "mesh", map[string]string{"den": den, "git": gitDir})
	syncTestFile(t, den, "a.txt", true)
	syncTestFile(t, gitDir, "b.txt", true)
	syncTestFile(t, gitDir, "wip.txt", false)
	chdirT(t, den)

	var out bytes.Buffer
	if err := runSyncCommand(ctx, []string{"--help"}, &out); err != nil || !strings.Contains(out.String(), "Usage: dendrite sync") {
		t.Fatalf("help: %v %s", err, out.String())
	}
	if err := runSyncCommand(ctx, []string{"--bogus"}, &out); err == nil {
		t.Fatal("expected flag error")
	}

	out.Reset()
	if err := runSyncCommand(ctx, []string{"--git-only", "--dry-run"}, &out); err != nil {
		t.Fatalf("dry run: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "would push 1 commit") || !strings.Contains(out.String(), "dendrite commit --synapse") {
		t.Errorf("dry-run table missing details:\n%s", out.String())
	}

	out.Reset()
	if err := runSyncCommand(ctx, []string{"--git-only", "--json"}, &out); err != nil {
		t.Fatalf("json run: %v\n%s", err, out.String())
	}
	var rep synapsesync.Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || len(rep.Members) != 2 {
		t.Fatalf("bad JSON report (%v): %s", err, out.String())
	}
	for dir, bare := range map[string]string{den: denBare, gitDir: gitBare} {
		if runGitT(t, bare, "rev-parse", "feature") != runGitT(t, dir, "rev-parse", "HEAD") {
			t.Errorf("%s not pushed", dir)
		}
	}

	// Plane enabled but unreachable (and no project bound): table renders Plane skip reasons.
	out.Reset()
	if err := runSyncCommand(ctx, nil, &out); err != nil {
		t.Fatalf("plane run: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "up to date") || !strings.Contains(out.String(), "no Plane project bound") {
		t.Errorf("unexpected table:\n%s", out.String())
	}

	// A missing member marks the run failed and flips the synapse.
	if err := os.RemoveAll(gitDir); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runSyncCommand(ctx, []string{"--git-only"}, &out); !errors.Is(err, errSyncFailed) {
		t.Fatalf("expected errSyncFailed, got %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "active -> broken") {
		t.Errorf("expected state transition in output:\n%s", out.String())
	}

	if err := runSyncCommand(ctx, []string{"--synapse", "nope"}, &out); err == nil || errors.Is(err, errSyncFailed) {
		t.Errorf("expected discovery error, got %v", err)
	}

	// Watch mode: runs passes (including failing ones) until the context ends.
	wctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	out.Reset()
	if err := runSyncCommand(wctx, []string{"--watch", "--interval", "50ms", "--git-only", "--json", "--synapse", "nope"}, &out); err != nil {
		t.Fatalf("watch: %v", err)
	}
	if strings.Count(out.String(), "sync pass") < 2 {
		t.Errorf("expected repeated passes:\n%s", out.String())
	}
	wctx2, cancel2 := context.WithTimeout(ctx, 3*time.Second)
	defer cancel2()
	out.Reset()
	_ = runSyncCommand(wctx2, []string{"--watch", "--interval", "1h", "--git-only"}, &out)
	if !strings.Contains(out.String(), "MEMBER") {
		t.Errorf("watch pass should print the table:\n%s", out.String())
	}
}

func TestSyncSummaries(t *testing.T) {
	cases := []struct {
		m    synapsesync.MemberReport
		git  string
		pl   string
		desc string
	}{
		{synapsesync.MemberReport{}, "-", "-", "empty"},
		{synapsesync.MemberReport{Push: &devtool.PushOutcome{Err: errors.New("x")}, Plane: &synapsesync.PlaneResult{Project: "DEN", StateChanges: 2, StatusPushed: true, Tickets: 3}},
			"push failed", "DEN: 2 state fix(es): 3 ticket(s) pushed", "failures"},
		{synapsesync.MemberReport{Push: &devtool.PushOutcome{Skipped: true, Reason: "r"}}, "skipped: r", "-", "skipped"},
	}
	for _, c := range cases {
		if got := gitSummary(c.m); got != c.git {
			t.Errorf("%s: gitSummary = %q, want %q", c.desc, got, c.git)
		}
		if got := planeSummary(c.m); got != c.pl {
			t.Errorf("%s: planeSummary = %q, want %q", c.desc, got, c.pl)
		}
	}
}
