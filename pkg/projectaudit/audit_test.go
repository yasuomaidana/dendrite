package projectaudit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

func gitRepo(t *testing.T, parent, name, remote string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmds := [][]string{{"init", "-q"}}
	if remote != "" {
		cmds = append(cmds, []string{"remote", "add", "origin", remote})
	}
	for _, args := range cmds {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAudit_RuleViolationsAndRelationships(t *testing.T) {
	base := t.TempDir()
	lib := gitRepo(t, base, "lib", "https://github.com/me/lib.git")
	app := gitRepo(t, base, "app", "git@github.com:me/app.git")
	svc := gitRepo(t, base, "svc", "https://github.com/me/svc.git")
	local := gitRepo(t, base, "scratch", "")
	vendor := gitRepo(t, base, "upstream", "https://github.com/someone/upstream.git")

	// app -> lib through a nested Cargo path dependency; svc -> lib through a go.mod require.
	write(t, filepath.Join(app, "crates", "core", "Cargo.toml"), "[dependencies]\nlib = { path = \"../../../lib\" }\n")
	write(t, filepath.Join(lib, "go.mod"), "module github.com/me/lib\n\ngo 1.22\n")
	write(t, filepath.Join(svc, "go.mod"), "module github.com/me/svc\n\nrequire (\n\tgithub.com/me/lib v0.1.0\n)\n")
	// Ignored: build output and a dependency on the non-editable upstream repo.
	write(t, filepath.Join(app, "target", "Cargo.toml"), "x = { path = \"../../svc\" }\n")
	write(t, filepath.Join(local, "package.json"), `{"dependencies": {"u": "file:../upstream"}}`)

	// A linked worktree is a working copy, not a project.
	write(t, filepath.Join(base, "copy", ".git"), "gitdir: /elsewhere\n")

	if err := plane.WriteLink(lib, plane.ProjectLink{Identifier: "LIB", ProjectID: "p-lib"}); err != nil {
		t.Fatal(err)
	}
	libID, err := projectid.Ensure(lib)
	if err != nil {
		t.Fatal(err)
	}
	svcID, err := projectid.Ensure(svc)
	if err != nil {
		t.Fatal(err)
	}
	syn := &synapse.Synapse{ID: "core", Members: []synapse.Member{
		{ProjectID: libID, Alias: "lib", PathHint: lib}, {ProjectID: svcID, Alias: "svc", PathHint: svc},
		{ProjectID: "dpid_gone", Alias: "app", PathHint: app}, // moved member: does not count as membership
	}}

	roots := Discover([]string{base, base})
	if len(roots) != 5 {
		t.Fatalf("Discover = %v; want the 5 repositories once each, without the worktree", roots)
	}
	rep := Audit(roots, []string{"ME"}, []*synapse.Synapse{syn})

	editable := map[string]bool{}
	for _, r := range rep.Repos {
		editable[r.Name] = r.Editable
	}
	if !editable["lib"] || !editable["app"] || !editable["scratch"] || editable["upstream"] {
		t.Errorf("editable = %v; owned and local repos are editable, third-party ones are not", editable)
	}
	_ = vendor

	missing := map[string]bool{}
	for _, r := range rep.MissingProject {
		missing[r.Name] = true
	}
	if len(missing) != 3 || !missing["app"] || !missing["svc"] || !missing["scratch"] {
		t.Errorf("MissingProject = %v; want app, svc, scratch", missing)
	}

	if len(rep.Edges) != 3 {
		t.Fatalf("Edges = %+v; want app->lib, scratch->upstream, svc->lib", rep.Edges)
	}
	if len(rep.Unsynapsed) != 1 || rep.Unsynapsed[0].From != app || rep.Unsynapsed[0].To != lib ||
		rep.Unsynapsed[0].Via != filepath.Join("crates", "core", "Cargo.toml") {
		t.Errorf("Unsynapsed = %+v; want only app->lib via crates/core/Cargo.toml", rep.Unsynapsed)
	}
}

func TestOwnerOfAndDiscoverMissingRoot(t *testing.T) {
	if got := OwnerOf("github.com/Acme/tool"); got != "acme" {
		t.Errorf("OwnerOf = %q", got)
	}
	if got := OwnerOf("tool"); got != "" {
		t.Errorf("OwnerOf(short) = %q", got)
	}
	if got := Discover([]string{filepath.Join(t.TempDir(), "absent")}); len(got) != 0 {
		t.Errorf("Discover(absent) = %v", got)
	}
}

func TestDependencies_ReplacesDuplicatesDepthAndUnreadableDirs(t *testing.T) {
	base := t.TempDir()
	lib := gitRepo(t, base, "lib", "")
	app := gitRepo(t, base, "app", "")

	// go.mod replace with an absolute path, plus a duplicate edge and a target outside every repository.
	write(t, filepath.Join(app, "go.mod"), "module app\n\nreplace example.com/lib => "+lib+"\n")
	write(t, filepath.Join(app, "pyproject.toml"), "[tool.uv.sources]\nlib = { path = \"../lib\" }\nother = { path = \"/nowhere/else\" }\n")
	// Manifests deeper than three levels are not scanned.
	write(t, filepath.Join(app, "a", "b", "c", "d", "Cargo.toml"), "x = { path = \"../../../../../lib\" }\n")
	// An unreadable directory is skipped rather than aborting the walk.
	locked := filepath.Join(app, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	edges := Dependencies(app, []string{lib, app})
	if len(edges) != 1 || edges[0].To != lib || edges[0].Via != "go.mod" {
		t.Errorf("edges = %+v; want one app->lib edge found first through go.mod", edges)
	}
}
