// Package selfupdate builds or downloads a new dendrite binary and swaps it in place
// with a backup and a smoke test, rolling back when the new binary does not start
// (FEAT-038). It also builds the binaries of local sibling repositories so develop
// mode picks up their changes.
package selfupdate

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrNoRelease is returned when the release source has no published release or no
// asset for this platform.
var ErrNoRelease = errors.New("selfupdate: no published release for this platform")

// Installer swaps a binary in place.
type Installer struct {
	// Target is the binary to replace.
	Target string
	// Smoke checks that a freshly installed binary runs; it defaults to `<bin> version`.
	Smoke func(ctx context.Context, bin string) error
}

// DefaultSmoke runs `<bin> version` with a timeout.
func DefaultSmoke(ctx context.Context, bin string) error {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, bin, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("smoke test `%s version` failed: %w: %s", bin, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Install moves staged (a new binary, ideally already in Target's directory) over
// Target: Target is first renamed to Target.bak, then staged is renamed into place and
// smoke-tested. On any failure Target.bak is restored. On success the backup is kept
// until the next install so a manual rollback stays possible.
func (in Installer) Install(ctx context.Context, staged string) error {
	if in.Target == "" {
		return errors.New("selfupdate: no install target")
	}
	smoke := in.Smoke
	if smoke == nil {
		smoke = DefaultSmoke
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		return fmt.Errorf("staging %s: %w", staged, err)
	}
	if err := os.MkdirAll(filepath.Dir(in.Target), 0o755); err != nil {
		return err
	}

	backup := in.Target + ".bak"
	hadTarget := false
	if _, err := os.Stat(in.Target); err == nil {
		hadTarget = true
		_ = os.Remove(backup)
		if err := os.Rename(in.Target, backup); err != nil {
			return fmt.Errorf("backing up %s: %w", in.Target, err)
		}
	}
	restore := func() {
		_ = os.Remove(in.Target)
		if hadTarget {
			_ = os.Rename(backup, in.Target)
		}
	}
	if err := os.Rename(staged, in.Target); err != nil {
		restore()
		return fmt.Errorf("installing %s: %w", in.Target, err)
	}
	if err := smoke(ctx, in.Target); err != nil {
		restore()
		return fmt.Errorf("%w (rolled back)", err)
	}
	return nil
}

// StagePath returns a temporary path next to target, so the final rename never
// crosses filesystems.
func StagePath(target string) string {
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".new")
}

// Run executes name args in dir, returning combined output in the error.
func Run(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s in %s: %w\n%s", name, strings.Join(args, " "), dir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// BuildGo compiles the main package pkg of the module in moduleDir to out.
func BuildGo(ctx context.Context, moduleDir, pkg, out string) error {
	return Run(ctx, moduleDir, "go", "build", "-o", out, pkg)
}

// ArtifactKind says how a member repository produces binaries.
type ArtifactKind string

const (
	KindGoBinary   ArtifactKind = "go-binary"
	KindRustBinary ArtifactKind = "rust-binary"
	KindLibrary    ArtifactKind = "library"
	KindUnknown    ArtifactKind = "unknown"
)

// Artifact is a buildable output of a member repository.
type Artifact struct {
	Kind ArtifactKind `json:"kind"`
	// Packages are the Go main packages (relative, e.g. "./cmd/x") for go-binary.
	Packages []string `json:"packages,omitempty"`
}

// Detect inspects dir: a Go module with main packages (module root or cmd/*), a Cargo
// crate with a binary target (src/main.rs, src/bin or [[bin]]), otherwise a library
// (go.mod or Cargo.toml without binaries) or unknown.
func Detect(dir string) Artifact {
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		if hasRustBinary(dir) {
			return Artifact{Kind: KindRustBinary}
		}
		return Artifact{Kind: KindLibrary}
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		var pkgs []string
		if isGoMain(dir) {
			pkgs = append(pkgs, ".")
		}
		entries, _ := os.ReadDir(filepath.Join(dir, "cmd"))
		for _, e := range entries {
			if e.IsDir() && isGoMain(filepath.Join(dir, "cmd", e.Name())) {
				pkgs = append(pkgs, "./cmd/"+e.Name())
			}
		}
		if len(pkgs) > 0 {
			return Artifact{Kind: KindGoBinary, Packages: pkgs}
		}
		return Artifact{Kind: KindLibrary}
	}
	return Artifact{Kind: KindUnknown}
}

func hasRustBinary(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "src", "main.rs")); err == nil {
		return true
	}
	if info, err := os.Stat(filepath.Join(dir, "src", "bin")); err == nil && info.IsDir() {
		return true
	}
	data, _ := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
	return strings.Contains(string(data), "[[bin]]")
}

// isGoMain reports whether dir holds a non-test Go file declaring package main.
func isGoMain(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "package ") {
				isMain := line == "package main"
				_ = f.Close()
				if isMain {
					return true
				}
				break
			}
		}
		_ = f.Close()
	}
	return false
}

// BuildMember builds dir's binaries: Go main packages into binDir (named after the
// package directory, the module directory for "."), Rust binaries with
// `cargo build --release` (left in target/release, where the sidecar locator looks).
// It returns what was built; libraries and unknown layouts build nothing.
func BuildMember(ctx context.Context, dir, binDir string) (Artifact, []string, error) {
	art := Detect(dir)
	switch art.Kind {
	case KindGoBinary:
		var built []string
		for _, pkg := range art.Packages {
			name := filepath.Base(pkg)
			if pkg == "." {
				name = filepath.Base(dir)
			}
			out := filepath.Join(binDir, name)
			if err := BuildGo(ctx, dir, pkg, out); err != nil {
				return art, built, err
			}
			built = append(built, out)
		}
		return art, built, nil
	case KindRustBinary:
		if err := Run(ctx, dir, "cargo", "build", "--release"); err != nil {
			return art, nil, err
		}
		return art, []string{filepath.Join(dir, "target", "release")}, nil
	}
	return art, nil, nil
}

// Release describes a published release and the asset for this platform.
type Release struct {
	Tag         string
	AssetURL    string
	ChecksumURL string
}

// AssetName is the release asset for goos/goarch, e.g. dendrite_darwin_arm64.
func AssetName(goos, goarch string) string { return "dendrite_" + goos + "_" + goarch }

// GitHubReleases fetches release metadata from the GitHub REST API.
type GitHubReleases struct {
	// Repo is "owner/name".
	Repo string
	// BaseURL defaults to https://api.github.com.
	BaseURL string
	Client  *http.Client
}

func (g GitHubReleases) client() *http.Client {
	if g.Client != nil {
		return g.Client
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// Latest returns the latest release's asset (and its .sha256) for the running platform.
func (g GitHubReleases) Latest(ctx context.Context) (*Release, error) {
	base := g.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/repos/"+g.Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := g.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying releases of %s: %w", g.Repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%s: %w", g.Repo, ErrNoRelease)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("querying releases of %s: HTTP %d", g.Repo, resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding release of %s: %w", g.Repo, err)
	}
	rel := &Release{Tag: body.TagName}
	want := AssetName(runtime.GOOS, runtime.GOARCH)
	for _, a := range body.Assets {
		switch a.Name {
		case want:
			rel.AssetURL = a.URL
		case want + ".sha256":
			rel.ChecksumURL = a.URL
		}
	}
	if rel.AssetURL == "" || rel.ChecksumURL == "" {
		return nil, fmt.Errorf("%s %s has no %s asset with checksum: %w", g.Repo, rel.Tag, want, ErrNoRelease)
	}
	return rel, nil
}

// Download fetches rel's asset to dest and verifies it against its SHA-256 checksum
// file (first field of the file); dest is removed on mismatch.
func (g GitHubReleases) Download(ctx context.Context, rel *Release, dest string) error {
	sumText, err := g.fetch(ctx, rel.ChecksumURL, io.Discard, true)
	if err != nil {
		return err
	}
	fields := strings.Fields(sumText)
	if len(fields) == 0 {
		return fmt.Errorf("empty checksum file at %s", rel.ChecksumURL)
	}
	want := strings.ToLower(fields[0])

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	h := sha256.New()
	if _, err := g.fetch(ctx, rel.AssetURL, io.MultiWriter(f, h), false); err != nil {
		_ = f.Close()
		_ = os.Remove(dest)
		return err
	}
	_ = f.Close()
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		_ = os.Remove(dest)
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", rel.AssetURL, got, want)
	}
	return nil
}

// fetch GETs url into w; with capture it returns the body as a string instead.
func (g GitHubReleases) fetch(ctx context.Context, url string, w io.Writer, capture bool) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := g.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: HTTP %d", url, resp.StatusCode)
	}
	if capture {
		data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return string(data), err
	}
	_, err = io.Copy(w, resp.Body)
	return "", err
}
