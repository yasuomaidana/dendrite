package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/selfupdate"
	"Dendrite/pkg/synapse"
)

func TestParseUpdateArgs(t *testing.T) {
	o, err := parseUpdateArgs([]string{"--dev", "--source-dir", "src", "--target=bin/d", "--synapse", "a", "--synapse=b", "--no-sync", "--no-members", "--autostash"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.dev || o.sourceDir != "src" || o.target != "bin/d" || strings.Join(o.synapses, ",") != "a,b" || !o.noSync || !o.noMembers || !o.autostash {
		t.Fatalf("opts = %+v", o)
	}
	for _, args := range [][]string{{"--target"}, {"--bogus"}, {"--synapse"}} {
		if _, err := parseUpdateArgs(args); err == nil {
			t.Errorf("%v: expected error", args)
		}
	}
	var out bytes.Buffer
	if err := runUpdateCommand(context.Background(), []string{"--help"}, &out); err != nil || !strings.Contains(out.String(), "--dev") {
		t.Fatalf("help = %q, %v", out.String(), err)
	}
	if err := runUpdateCommand(context.Background(), []string{"--bogus"}, &out); err == nil {
		t.Fatal("expected flag error")
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolveDendriteSource(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good")
	writeTree(t, good, map[string]string{"go.mod": "module Dendrite\n", "cmd/dendrite/main.go": "package main\n"})
	if got, err := resolveDendriteSource(good); err != nil || got != good {
		t.Fatalf("good = %q, %v", got, err)
	}
	t.Setenv("DENDRITE_SRC", good)
	if got, err := resolveDendriteSource(""); err != nil || got != good {
		t.Fatalf("env = %q, %v", got, err)
	}
	t.Setenv("DENDRITE_SRC", "")

	other := filepath.Join(root, "other")
	writeTree(t, other, map[string]string{"go.mod": "module example.com/other\n"})
	if _, err := resolveDendriteSource(other); err == nil || !strings.Contains(err.Error(), "not a Dendrite checkout") {
		t.Fatalf("other err = %v", err)
	}
	nocmd := filepath.Join(root, "nocmd")
	writeTree(t, nocmd, map[string]string{"go.mod": "module Dendrite\n"})
	if _, err := resolveDendriteSource(nocmd); err == nil || !strings.Contains(err.Error(), "cmd/dendrite") {
		t.Fatalf("nocmd err = %v", err)
	}

	// Default: the enclosing git checkout.
	gitRun(t, good, "init", "-q")
	t.Chdir(filepath.Join(good, "cmd"))
	if got, err := resolveDendriteSource(""); err != nil || filepath.Base(got) != "good" {
		t.Fatalf("toplevel = %q, %v", got, err)
	}
	t.Chdir(other)
	if _, err := resolveDendriteSource(""); err == nil {
		t.Fatal("expected error outside a git checkout")
	}
	if goModModule(filepath.Join(root, "missing", "go.mod")) != "" {
		t.Fatal("goModModule on missing file")
	}
}

func TestResolveUpdateTarget(t *testing.T) {
	if got, err := resolveUpdateTarget("rel/dendrite"); err != nil || !filepath.IsAbs(got) {
		t.Fatalf("explicit = %q, %v", got, err)
	}
	// The test binary is not named dendrite, so the Go install dir is used.
	gobin := t.TempDir()
	t.Setenv("GOBIN", gobin)
	if got, err := resolveUpdateTarget(""); err != nil || got != filepath.Join(gobin, "dendrite") {
		t.Fatalf("GOBIN = %q, %v", got, err)
	}
	t.Setenv("GOBIN", "")
	if got, err := resolveUpdateTarget(""); err != nil || !strings.HasSuffix(got, filepath.Join("bin", "dendrite")) {
		t.Fatalf("GOPATH = %q, %v", got, err)
	}
	t.Setenv("PATH", t.TempDir()) // no go binary
	if _, err := resolveUpdateTarget(""); err == nil || !strings.Contains(err.Error(), "--target") {
		t.Fatalf("no go err = %v", err)
	}
}

func TestFormatRevision(t *testing.T) {
	cases := []struct {
		settings []debug.BuildSetting
		want     string
	}{
		{nil, ""},
		{[]debug.BuildSetting{{Key: "vcs.revision", Value: "0123456789"}}, "commit 0123456"},
		{[]debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.time", Value: "T"}, {Key: "vcs.modified", Value: "true"}}, "commit abc (T, modified)"},
	}
	for _, c := range cases {
		if got := formatRevision(c.settings); got != c.want {
			t.Errorf("formatRevision(%v) = %q, want %q", c.settings, got, c.want)
		}
	}
	_ = buildRevision()
}

func TestRunReleaseUpdate(t *testing.T) {
	ctx := context.Background()
	target := filepath.Join(t.TempDir(), "dendrite")
	none := httptest.NewServer(http.NotFoundHandler())
	defer none.Close()
	err := runReleaseUpdate(ctx, selfupdate.GitHubReleases{Repo: "o/d", BaseURL: none.URL}, target, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "update --dev") {
		t.Fatalf("no release err = %v", err)
	}

	payload := []byte("#!/bin/sh\nexit 0\n")
	sum := sha256.Sum256(payload)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := selfupdate.AssetName(runtime.GOOS, runtime.GOARCH)
		switch r.URL.Path {
		case "/repos/o/d/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v9","assets":[{"name":%q,"browser_download_url":%q},{"name":%q,"browser_download_url":%q}]}`,
				name, srv.URL+"/bin", name+".sha256", srv.URL+"/sum")
		case "/bin":
			_, _ = w.Write(payload)
		case "/sum":
			fmt.Fprint(w, hex.EncodeToString(sum[:]))
		case "/bad":
			fmt.Fprint(w, strings.Repeat("0", 64))
		}
	}))
	defer srv.Close()
	var out bytes.Buffer
	gh := selfupdate.GitHubReleases{Repo: "o/d", BaseURL: srv.URL}
	if err := runReleaseUpdate(ctx, gh, target, &out); err != nil || !strings.Contains(out.String(), "v9 installed") {
		t.Fatalf("release update = %q, %v", out.String(), err)
	}

	// HTTP errors other than 404 surface as-is.
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer broken.Close()
	if err := runReleaseUpdate(ctx, selfupdate.GitHubReleases{Repo: "o/d", BaseURL: broken.URL}, target, &out); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("502 err = %v", err)
	}
	// A staged binary that fails the smoke test is rolled back.
	failing := []byte("#!/bin/sh\nexit 1\n")
	fsum := sha256.Sum256(failing)
	var fsrv *httptest.Server
	fsrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := selfupdate.AssetName(runtime.GOOS, runtime.GOARCH)
		switch r.URL.Path {
		case "/repos/o/d/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v10","assets":[{"name":%q,"browser_download_url":%q},{"name":%q,"browser_download_url":%q}]}`,
				name, fsrv.URL+"/bin", name+".sha256", fsrv.URL+"/sum")
		case "/bin":
			_, _ = w.Write(failing)
		case "/sum":
			fmt.Fprint(w, hex.EncodeToString(fsum[:]))
		}
	}))
	defer fsrv.Close()
	if err := runReleaseUpdate(ctx, selfupdate.GitHubReleases{Repo: "o/d", BaseURL: fsrv.URL}, target, &out); err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("smoke failure err = %v", err)
	}
	if data, _ := os.ReadFile(target); string(data) != string(payload) {
		t.Fatal("target not restored after failed smoke test")
	}
	// Checksum mismatch.
	bad := &selfupdate.Release{AssetURL: srv.URL + "/bin", ChecksumURL: srv.URL + "/bad"}
	if err := gh.Download(ctx, bad, filepath.Join(t.TempDir(), "x")); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

// devWorld builds a tiny Dendrite checkout and a sibling Go tool, each cloned from a
// bare origin, linked by a synapse.
type devWorld struct {
	src, tool, toolSeed, target string
}

func newDevWorld(t *testing.T) devWorld {
	t.Helper()
	for k, v := range map[string]string{
		"DENDRITE_HOME": t.TempDir(), "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
		"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
	} {
		t.Setenv(k, v)
	}
	root := t.TempDir()
	repo := func(name string, files map[string]string) (string, string) {
		origin, seed, clone := filepath.Join(root, name+".git"), filepath.Join(root, "seed-"+name), filepath.Join(root, name)
		gitRun(t, root, "init", "-q", "--bare", "-b", "develop", origin)
		gitRun(t, root, "clone", "-q", origin, seed)
		gitRun(t, seed, "checkout", "-q", "-b", "develop")
		writeTree(t, seed, files)
		gitRun(t, seed, "add", "-A")
		gitRun(t, seed, "commit", "-q", "-m", "init")
		gitRun(t, seed, "push", "-q", "-u", "origin", "develop")
		gitRun(t, root, "clone", "-q", "-b", "develop", origin, clone)
		return clone, seed
	}
	src, _ := repo("Dendrite", map[string]string{
		"go.mod":               "module Dendrite\n\ngo 1.22\n",
		"cmd/dendrite/main.go": "package main\n\nfunc main() {}\n",
	})
	tool, toolSeed := repo("tool", map[string]string{
		"go.mod":  "module example.com/tool\n\ngo 1.22\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})
	return devWorld{src: src, tool: tool, toolSeed: toolSeed, target: filepath.Join(root, "install", "dendrite")}
}

func TestRunDevUpdateEndToEnd(t *testing.T) {
	w := newDevWorld(t)
	ctx := context.Background()
	var out bytes.Buffer

	// Not in any synapse yet: only dendrite is rebuilt.
	if err := runUpdateCommand(ctx, []string{"--dev", "--source-dir", w.src, "--target", w.target}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "belongs to no synapse") || !strings.Contains(out.String(), "dendrite rebuilt") {
		t.Fatalf("output = %s", out.String())
	}

	if err := runSynapseCommand(ctx, []string{"create", "stack", "--members", w.src + "=control:consumer," + w.tool + "=tool:producer", "--sync", "go.work"}, &out); err != nil {
		t.Fatal(err)
	}
	// Publish a new tool commit; the update must fast-forward and rebuild it.
	writeTree(t, w.toolSeed, map[string]string{"extra.go": "package main\n"})
	gitRun(t, w.toolSeed, "add", "-A")
	gitRun(t, w.toolSeed, "commit", "-q", "-m", "extra")
	gitRun(t, w.toolSeed, "push", "-q", "origin", "develop")

	out.Reset()
	if err := runUpdateCommand(ctx, []string{"--dev", "--source-dir", w.src, "--target", w.target}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(w.tool, "extra.go")); err != nil {
		t.Fatalf("tool not fast-forwarded:\n%s", out.String())
	}
	home, _ := projectid.HomeDir()
	if _, err := os.Stat(filepath.Join(home, "bin", "tool")); err != nil {
		t.Fatalf("tool binary not built:\n%s", out.String())
	}
	if data, _ := os.ReadFile(filepath.Join(w.src, "go.work")); !strings.Contains(string(data), "../tool") {
		t.Fatalf("go.work = %q", data)
	}
	if _, err := os.Stat(w.target + ".bak"); err != nil {
		t.Fatal("previous binary not kept")
	}
	if out := runBin(t, w.target); out != "" {
		t.Fatalf("installed binary output = %q", out)
	}

	// A failing member build is reported but does not block the dendrite rebuild;
	// --no-sync skips the fast-forward.
	writeTree(t, w.tool, map[string]string{"broken.go": "package main\n\nvar _ = undefined\n"})
	out.Reset()
	if err := runUpdateCommand(ctx, []string{"--dev", "--source-dir", w.src, "--target", w.target, "--no-sync", "--synapse", "stack"}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "tool: build failed") || !strings.Contains(out.String(), "[!] building tool") {
		t.Fatalf("output = %s", out.String())
	}

	// A broken dendrite build fails the update and leaves the installed binary alone.
	writeTree(t, w.src, map[string]string{"cmd/dendrite/broken.go": "package main\n\nvar _ = undefined\n"})
	if err := runUpdateCommand(ctx, []string{"--dev", "--source-dir", w.src, "--target", w.target, "--no-sync", "--no-members"}, &out); err == nil {
		t.Fatal("expected build failure")
	}
	if _, err := os.Stat(selfupdate.StagePath(w.target)); !os.IsNotExist(err) {
		t.Fatal("staged binary left behind")
	}

	if _, err := updateSynapses(w.src, []string{"nope"}); err == nil {
		t.Fatal("expected unknown synapse error")
	}
	if err := runUpdateCommand(ctx, []string{"--dev", "--source-dir", t.TempDir(), "--target", w.target}, &out); err == nil {
		t.Fatal("expected invalid source error")
	}
}

func TestUpdateSynapsesSkipsArchivedAndReportsMemberErrors(t *testing.T) {
	w := newDevWorld(t)
	ctx := context.Background()
	var out bytes.Buffer
	if err := runSynapseCommand(ctx, []string{"create", "old", "--members", w.src + "=control:consumer," + w.tool + "=tool:producer"}, &out); err != nil {
		t.Fatal(err)
	}
	st, _ := synapse.DefaultStore()
	s, _ := st.Load("old")
	if err := s.Transition(synapse.StateArchived); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
	if syns, err := updateSynapses(w.src, nil); err != nil || len(syns) != 0 {
		t.Fatalf("archived synapse selected: %v, %v", syns, err)
	}

	// A member whose checkout vanished shows up as a warning.
	if err := runSynapseCommand(ctx, []string{"create", "live", "--members", w.src + "=control:consumer," + w.tool + "=tool:producer"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(w.tool); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runUpdateCommand(ctx, []string{"--dev", "--source-dir", w.src, "--target", w.target, "--no-members"}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "[!] live/tool") {
		t.Fatalf("output = %s", out.String())
	}
}

func runBin(t *testing.T, bin string) string {
	t.Helper()
	out, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", bin, err, out)
	}
	return strings.TrimSpace(string(out))
}
