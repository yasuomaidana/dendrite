package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/tracker/plane"
)

func migrateGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestSomaMigrateState_UntracksDendriteAndRecordsIdentity(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	repo := t.TempDir()
	migrateGit(t, repo, "init", "-q")
	if err := plane.WriteLink(repo, plane.ProjectLink{Identifier: "APP", ProjectID: "uuid-1", Workspace: "ws"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("pkg/**/.dendrite/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ixPath, _ := projectid.DefaultIndexPath()
	ix, _ := projectid.LoadIndex(ixPath)
	if _, err := ix.Register(repo); err != nil {
		t.Fatal(err)
	}
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	migrateGit(t, repo, "add", "-f", ".dendrite", ".gitignore")
	migrateGit(t, repo, "commit", "-q", "-m", "root")

	var out bytes.Buffer
	if err := runSomaMigrateState(false, &out); err != nil {
		t.Fatalf("migrate-state: %v\n%s", err, out.String())
	}

	if tracked := strings.TrimSpace(migrateGit(t, repo, "ls-files", ".dendrite")); tracked != "" {
		t.Fatalf(".dendrite is still tracked: %q", tracked)
	}
	for _, f := range []string{"project.id", "plane.json"} {
		if _, err := os.Stat(filepath.Join(repo, ".dendrite", f)); err != nil {
			t.Fatalf("%s must stay on disk: %v", f, err)
		}
	}
	gi, _ := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if !strings.Contains(string(gi), "\n.dendrite/\n") {
		t.Fatalf(".gitignore lacks the .dendrite/ line (a pkg/**/.dendrite/ rule must not count): %q", gi)
	}
	if log := strings.TrimSpace(migrateGit(t, repo, "log", "--oneline")); strings.Count(log, "\n") != 0 {
		t.Fatalf("migrate-state must not commit, log: %s", log)
	}

	ix, _ = projectid.LoadIndex(ixPath)
	e, ok := ix.LookupByPath(repo)
	if !ok || e.RootCommit == "" || e.Plane == nil || e.Plane.Identifier != "APP" {
		t.Fatalf("index entry lacks root commit / plane binding: %+v", e)
	}

	// Idempotent: a second run changes nothing and does not append another ignore line.
	out.Reset()
	if err := runSomaMigrateState(false, &out); err != nil {
		t.Fatal(err)
	}
	gi2, _ := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if string(gi2) != string(gi) {
		t.Fatalf("second run rewrote .gitignore:\n%s", gi2)
	}
}

func TestSomaMigrateState_SkipsMissingPathsAndHonoursDryRun(t *testing.T) {
	t.Setenv("DENDRITE_HOME", t.TempDir())
	ixPath, _ := projectid.DefaultIndexPath()
	ix, _ := projectid.LoadIndex(ixPath)
	ix.Upsert(projectid.Entry{ID: "dpid_" + strings.Repeat("a", 32), Path: filepath.Join(t.TempDir(), "gone"), Name: "gone"})
	_ = ix.Save()

	var out bytes.Buffer
	if err := runSomaMigrateState(true, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "skipped: path is missing") || !strings.Contains(out.String(), "dry run") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}
