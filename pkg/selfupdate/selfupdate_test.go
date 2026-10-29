package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeExec(t *testing.T, path, script string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInstallSwapsAndKeepsBackup(t *testing.T) {
	dir := t.TempDir()
	target := writeExec(t, filepath.Join(dir, "dendrite"), "echo old\n")
	staged := writeExec(t, StagePath(target), "echo new\n")
	if err := (Installer{Target: target}).Install(context.Background(), staged); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(target); !strings.Contains(string(data), "echo new") {
		t.Fatalf("target = %s", data)
	}
	if data, _ := os.ReadFile(target + ".bak"); !strings.Contains(string(data), "echo old") {
		t.Fatalf("backup = %s", data)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatal("staged file should be consumed")
	}
}

func TestInstallRollsBackFailedSmokeTest(t *testing.T) {
	dir := t.TempDir()
	target := writeExec(t, filepath.Join(dir, "dendrite"), "echo old\n")
	staged := writeExec(t, StagePath(target), "echo broken >&2\nexit 1\n")
	err := (Installer{Target: target}).Install(context.Background(), staged)
	if err == nil || !strings.Contains(err.Error(), "rolled back") || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("err = %v", err)
	}
	if data, _ := os.ReadFile(target); !strings.Contains(string(data), "echo old") {
		t.Fatalf("target not restored: %s", data)
	}
}

func TestInstallFreshTargetAndErrors(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	target := filepath.Join(dir, "bin", "dendrite")
	staged := writeExec(t, filepath.Join(dir, "new"), "exit 0\n")
	if err := (Installer{Target: target}).Install(ctx, staged); err != nil {
		t.Fatalf("fresh install: %v", err)
	}

	// Failed smoke on a fresh install leaves no target behind.
	fresh := filepath.Join(dir, "other", "dendrite")
	staged = writeExec(t, filepath.Join(dir, "new2"), "exit 0\n")
	fail := func(context.Context, string) error { return errors.New("nope") }
	if err := (Installer{Target: fresh, Smoke: fail}).Install(ctx, staged); err == nil {
		t.Fatal("expected smoke failure")
	}
	if _, err := os.Stat(fresh); !os.IsNotExist(err) {
		t.Fatal("failed fresh install left a target")
	}

	if err := (Installer{}).Install(ctx, staged); err == nil {
		t.Fatal("expected missing target error")
	}
	if err := (Installer{Target: target}).Install(ctx, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected staging error")
	}
	// Target's parent is a file.
	blocker := filepath.Join(dir, "blocker")
	writeFile(t, blocker, "")
	staged = writeExec(t, filepath.Join(dir, "new3"), "exit 0\n")
	if err := (Installer{Target: filepath.Join(blocker, "dendrite")}).Install(ctx, staged); err == nil {
		t.Fatal("expected mkdir error")
	}
	// Rename into a directory target fails and restores the backup.
	dirTarget := filepath.Join(dir, "dirtarget")
	writeFile(t, filepath.Join(dirTarget, "keep"), "x")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := (Installer{Target: dirTarget}).Install(ctx, staged); err == nil {
		t.Fatal("expected backup error in read-only dir")
	}
}

func TestInstallRenameFailureRestoresBackup(t *testing.T) {
	dir := t.TempDir()
	target := writeExec(t, filepath.Join(dir, "dendrite"), "echo old\n")
	// A staged path in another, read-only directory cannot be renamed away.
	roDir := filepath.Join(dir, "ro")
	staged := writeExec(t, filepath.Join(roDir, "new"), "exit 0\n")
	if err := os.Chmod(roDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })
	if err := (Installer{Target: target}).Install(context.Background(), staged); err == nil || !strings.Contains(err.Error(), "installing") {
		t.Fatalf("err = %v", err)
	}
	if data, _ := os.ReadFile(target); !strings.Contains(string(data), "echo old") {
		t.Fatalf("target not restored: %s", data)
	}
}

func TestDefaultSmoke(t *testing.T) {
	dir := t.TempDir()
	if err := DefaultSmoke(context.Background(), writeExec(t, filepath.Join(dir, "ok"), "[ \"$1\" = version ]\n")); err != nil {
		t.Fatal(err)
	}
	if err := DefaultSmoke(context.Background(), writeExec(t, filepath.Join(dir, "bad"), "exit 2\n")); err == nil {
		t.Fatal("expected smoke failure")
	}
}

func TestDetect(t *testing.T) {
	root := t.TempDir()
	cases := map[string]struct {
		files map[string]string
		want  Artifact
	}{
		"gomain":      {map[string]string{"go.mod": "module m\n", "main.go": "// c\npackage main\n", "x_test.go": "package main\n"}, Artifact{Kind: KindGoBinary, Packages: []string{"."}}},
		"gocmd":       {map[string]string{"go.mod": "module m\n", "lib.go": "package lib\n", "cmd/tool/main.go": "package main\n", "cmd/notmain/x.go": "package x\n", "cmd/file.go": "package main\n"}, Artifact{Kind: KindGoBinary, Packages: []string{"./cmd/tool"}}},
		"golib":       {map[string]string{"go.mod": "module m\n", "lib.go": "package lib\n"}, Artifact{Kind: KindLibrary}},
		"rsmain":      {map[string]string{"Cargo.toml": "[package]\n", "src/main.rs": ""}, Artifact{Kind: KindRustBinary}},
		"rsbindir":    {map[string]string{"Cargo.toml": "[package]\n", "src/bin/x.rs": ""}, Artifact{Kind: KindRustBinary}},
		"rsbin":       {map[string]string{"Cargo.toml": "[package]\n[[bin]]\nname = \"x\"\n"}, Artifact{Kind: KindRustBinary}},
		"rsworkspace": {map[string]string{"Cargo.toml": "[workspace]\nmembers = [\"crates/a\"]\n", "crates/a/Cargo.toml": "[package]\n[[bin]]\nname = \"x\"\n"}, Artifact{Kind: KindRustBinary}},
		"rslib":       {map[string]string{"Cargo.toml": "[package]\n", "src/lib.rs": ""}, Artifact{Kind: KindLibrary}},
		"empty":       {nil, Artifact{Kind: KindUnknown}},
	}
	for name, c := range cases {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for f, content := range c.files {
			writeFile(t, filepath.Join(dir, f), content)
		}
		got := Detect(dir)
		if got.Kind != c.want.Kind || strings.Join(got.Packages, ",") != strings.Join(c.want.Packages, ",") {
			t.Errorf("%s: Detect = %+v, want %+v", name, got, c.want)
		}
	}
	// An unreadable Go file is skipped.
	dir := filepath.Join(root, "unreadable")
	writeFile(t, filepath.Join(dir, "go.mod"), "module m\n")
	writeFile(t, filepath.Join(dir, "a.go"), "package main\n")
	if err := os.Chmod(filepath.Join(dir, "a.go"), 0o000); err != nil {
		t.Fatal(err)
	}
	if got := Detect(dir); got.Kind != KindLibrary {
		t.Errorf("unreadable main = %+v", got)
	}
}

func TestBuildMember(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")

	gomod := filepath.Join(root, "tool")
	writeFile(t, filepath.Join(gomod, "go.mod"), "module example.com/tool\n\ngo 1.22\n")
	writeFile(t, filepath.Join(gomod, "main.go"), "package main\n\nfunc main() {}\n")
	writeFile(t, filepath.Join(gomod, "cmd", "helper", "main.go"), "package main\n\nfunc main() {}\n")
	art, built, err := BuildMember(ctx, gomod, binDir)
	if err != nil || art.Kind != KindGoBinary || len(built) != 2 {
		t.Fatalf("go build = %+v %v %v", art, built, err)
	}
	for _, b := range []string{"tool", "helper"} {
		if _, err := os.Stat(filepath.Join(binDir, b)); err != nil {
			t.Fatalf("missing %s: %v", b, err)
		}
	}

	broken := filepath.Join(root, "broken")
	writeFile(t, filepath.Join(broken, "go.mod"), "module example.com/broken\n")
	writeFile(t, filepath.Join(broken, "main.go"), "package main\n\nfunc main() { undefined() }\n")
	if _, _, err := BuildMember(ctx, broken, binDir); err == nil || !strings.Contains(err.Error(), "go build") {
		t.Fatalf("broken go err = %v", err)
	}

	// cargo is shimmed on PATH so the test does not need a Rust toolchain.
	shims := filepath.Join(root, "shims")
	writeExec(t, filepath.Join(shims, "cargo"), "[ -f FAIL ] && { echo boom >&2; exit 1; }\nmkdir -p target/release && touch target/release/crate\n")
	t.Setenv("PATH", shims+string(os.PathListSeparator)+os.Getenv("PATH"))
	crate := filepath.Join(root, "crate")
	writeFile(t, filepath.Join(crate, "Cargo.toml"), "[package]\nname = \"crate\"\n")
	writeFile(t, filepath.Join(crate, "src", "main.rs"), "fn main() {}\n")
	art, built, err = BuildMember(ctx, crate, binDir)
	if err != nil || art.Kind != KindRustBinary || built[0] != filepath.Join(crate, "target", "release") {
		t.Fatalf("cargo build = %+v %v %v", art, built, err)
	}
	writeFile(t, filepath.Join(crate, "FAIL"), "")
	if _, _, err := BuildMember(ctx, crate, binDir); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("cargo failure = %v", err)
	}

	lib := filepath.Join(root, "lib")
	writeFile(t, filepath.Join(lib, "go.mod"), "module example.com/lib\n")
	if art, built, err := BuildMember(ctx, lib, binDir); err != nil || art.Kind != KindLibrary || built != nil {
		t.Fatalf("library = %+v %v %v", art, built, err)
	}
}

// releaseServer serves a GitHub-like releases API with one asset for this platform.
func releaseServer(t *testing.T, payload []byte, sum string, status int, assets bool) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/dendrite/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		name := AssetName(runtime.GOOS, runtime.GOARCH)
		list := "[]"
		if assets {
			list = fmt.Sprintf(`[{"name":%q,"browser_download_url":%q},{"name":%q,"browser_download_url":%q},{"name":"other","browser_download_url":"x"}]`,
				name, srv.URL+"/asset", name+".sha256", srv.URL+"/asset.sha256")
		}
		fmt.Fprintf(w, `{"tag_name":"v1.4.0","assets":%s}`, list)
	})
	mux.HandleFunc("/asset", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) })
	mux.HandleFunc("/asset.sha256", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, sum) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGitHubReleasesLatestAndDownload(t *testing.T) {
	ctx := context.Background()
	payload := []byte("#!/bin/sh\nexit 0\n")
	h := sha256.Sum256(payload)
	sum := hex.EncodeToString(h[:]) + "  " + AssetName(runtime.GOOS, runtime.GOARCH) + "\n"
	srv := releaseServer(t, payload, sum, http.StatusOK, true)
	gh := GitHubReleases{Repo: "o/dendrite", BaseURL: srv.URL}

	rel, err := gh.Latest(ctx)
	if err != nil || rel.Tag != "v1.4.0" || rel.AssetURL != srv.URL+"/asset" {
		t.Fatalf("latest = %+v, %v", rel, err)
	}
	dest := filepath.Join(t.TempDir(), "dendrite.new")
	if err := gh.Download(ctx, rel, dest); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(dest); string(data) != string(payload) {
		t.Fatalf("downloaded = %q", data)
	}

	// Checksum mismatch removes the download.
	bad := releaseServer(t, payload, strings.Repeat("0", 64), http.StatusOK, true)
	badGH := GitHubReleases{Repo: "o/dendrite", BaseURL: bad.URL}
	rel, _ = badGH.Latest(ctx)
	if err := badGH.Download(ctx, rel, dest); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("mismatch err = %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("mismatched download not removed")
	}
	// Empty checksum file.
	empty := releaseServer(t, payload, "  ", http.StatusOK, true)
	emptyGH := GitHubReleases{Repo: "o/dendrite", BaseURL: empty.URL}
	rel, _ = emptyGH.Latest(ctx)
	if err := emptyGH.Download(ctx, rel, dest); err == nil || !strings.Contains(err.Error(), "empty checksum") {
		t.Fatalf("empty checksum err = %v", err)
	}
	// Asset URL failing and dest not creatable.
	rel = &Release{AssetURL: srv.URL + "/missing", ChecksumURL: srv.URL + "/asset.sha256"}
	if err := gh.Download(ctx, rel, dest); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("asset 404 err = %v", err)
	}
	rel = &Release{AssetURL: srv.URL + "/asset", ChecksumURL: srv.URL + "/asset.sha256"}
	if err := gh.Download(ctx, rel, filepath.Join(t.TempDir(), "no", "dir", "x")); err == nil {
		t.Fatal("expected create error")
	}
	rel = &Release{ChecksumURL: srv.URL + "/missing"}
	if err := gh.Download(ctx, rel, dest); err == nil {
		t.Fatal("expected checksum 404")
	}
}

func TestGitHubReleasesErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := (GitHubReleases{Repo: "o/dendrite", BaseURL: releaseServer(t, nil, "", http.StatusNotFound, false).URL}).Latest(ctx); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("404 err = %v", err)
	}
	if _, err := (GitHubReleases{Repo: "o/dendrite", BaseURL: releaseServer(t, nil, "", http.StatusInternalServerError, false).URL}).Latest(ctx); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("500 err = %v", err)
	}
	if _, err := (GitHubReleases{Repo: "o/dendrite", BaseURL: releaseServer(t, nil, "", http.StatusOK, false).URL}).Latest(ctx); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("no assets err = %v", err)
	}
	garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "{") }))
	defer garbage.Close()
	if _, err := (GitHubReleases{Repo: "o/dendrite", BaseURL: garbage.URL}).Latest(ctx); err == nil || !strings.Contains(err.Error(), "decoding") {
		t.Fatalf("garbage err = %v", err)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	if _, err := (GitHubReleases{Repo: "o/dendrite", BaseURL: closed.URL}).Latest(ctx); err == nil {
		t.Fatal("expected connection error")
	}
	if _, err := (GitHubReleases{Repo: "o/dendrite", BaseURL: "://bad"}).Latest(ctx); err == nil {
		t.Fatal("expected request error")
	}
	if _, err := (GitHubReleases{}).fetch(ctx, "://bad", nil, true); err == nil {
		t.Fatal("expected fetch request error")
	}
	if _, err := (GitHubReleases{}).fetch(ctx, closed.URL, nil, true); err == nil {
		t.Fatal("expected fetch connection error")
	}
	if (GitHubReleases{}).client() == nil || (GitHubReleases{Client: http.DefaultClient}).client() != http.DefaultClient {
		t.Fatal("client selection")
	}
	if AssetName("linux", "amd64") != "dendrite_linux_amd64" {
		t.Fatal("AssetName")
	}
}

// offlineTransport fails every request, reporting the URL it was asked for.
type offlineTransport struct{}

func (offlineTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, errors.New("offline: " + r.URL.String())
}

func TestLatestDefaultsToGitHubAPI(t *testing.T) {
	gh := GitHubReleases{Repo: "o/dendrite", Client: &http.Client{Transport: offlineTransport{}}}
	if _, err := gh.Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "https://api.github.com/repos/o/dendrite/releases/latest") {
		t.Fatalf("err = %v", err)
	}
	if isGoMain(filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("missing dir reported as main package")
	}
}
