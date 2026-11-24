package engine

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed dockerfiles/*.Dockerfile dockerfiles/ghidra/*.java
var dockerfiles embed.FS

// Availability says whether an engine can run in one mode and, when it cannot, why and how to fix it.
type Availability struct {
	Usable bool   `json:"usable"`
	Reason string `json:"reason,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

// Status is an engine of the catalog and where it can run on this machine.
type Status struct {
	Spec      Spec         `json:"spec"`
	Container Availability `json:"container"`
	Host      Availability `json:"host"`
}

// availability reports spec in mode on this machine.
func availability(ctx context.Context, spec Spec, mode string, env Env) Availability {
	if !spec.Implemented {
		return Availability{Reason: spec.Note}
	}
	if _, problem := usable(ctx, spec, mode, env); problem != nil {
		return Availability{Reason: problem.Reason, Fix: problem.Fix}
	}
	return Availability{Usable: true}
}

// Statuses lists every engine of the catalog with where it can run on this machine, for `dendrite retro engines`.
func Statuses(ctx context.Context, env Env) []Status {
	var out []Status
	for _, spec := range Catalog() {
		out = append(out, Status{Spec: spec, Container: availability(ctx, spec, ModeContainer, env), Host: availability(ctx, spec, ModeHost, env)})
	}
	return out
}

// BuildOptions are what an engine image needs from the user. Ghidra's release is large and not pinned by Dendrite:
// the user chooses it (URL) and says what it must hash to (SHA256), and the build verifies the download.
type BuildOptions struct {
	URL    string
	SHA256 string
}

var sha256Hex = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// buildContextFiles are the embedded files an engine's image build needs besides its Dockerfile.
var buildContextFiles = map[string][]string{"ghidra": {"dockerfiles/ghidra/" + ghidraScript}}

// buildArgs checks the options of an engine and returns its `--build-arg` flags (none for engines that need none).
func buildArgs(spec Spec, o BuildOptions) ([]string, error) {
	if spec.Family != FamilyGhidra {
		return nil, nil
	}
	if !strings.HasPrefix(o.URL, "https://") || !sha256Hex.MatchString(o.SHA256) {
		return nil, errors.New("Ghidra's release is not pinned by Dendrite: pass --url (an https link to the release zip you chose) and --sha256 (its 64-hex-digit checksum)")
	}
	return []string{"--build-arg", "GHIDRA_URL=" + o.URL, "--build-arg", "GHIDRA_SHA256=" + o.SHA256}, nil
}

// Build builds the container image of an engine from the Dockerfile embedded in this package. It is explicit: no
// analysis builds or pulls an image by itself. It returns what `docker build` printed.
func (s *Service) Build(ctx context.Context, name string, o BuildOptions) (string, error) {
	spec, ok := Lookup(name)
	if !ok || !spec.Implemented {
		return "", fmt.Errorf("no engine image to build for %q: build one of %v", name, implementedNames())
	}
	extra, err := buildArgs(spec, o)
	if err != nil {
		return "", err
	}
	text, _ := dockerfiles.ReadFile("dockerfiles/" + spec.Name + ".Dockerfile") // every implemented engine has one (tested)
	dir, err := os.MkdirTemp("", "rea-engine-build")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	_ = os.WriteFile(filepath.Join(dir, "Dockerfile"), text, 0o644) // a file that was not written makes docker report the missing Dockerfile
	for _, f := range buildContextFiles[spec.Name] {
		data, _ := dockerfiles.ReadFile(f)
		_ = os.WriteFile(filepath.Join(dir, filepath.Base(f)), data, 0o644)
	}
	args := append([]string{"build", "-t", spec.Image}, extra...)
	out, err := s.exec().Run(ctx, "docker", append(args, dir)...)
	return string(out), err
}
