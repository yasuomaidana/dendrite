package engine

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed dockerfiles/*.Dockerfile
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
		return Availability{Reason: "planned, not implemented yet"}
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

// Build builds the container image of an engine from the Dockerfile embedded in this package. It is explicit: no
// analysis builds or pulls an image by itself. It returns what `docker build` printed.
func (s *Service) Build(ctx context.Context, name string) (string, error) {
	spec, ok := Lookup(name)
	if !ok || !spec.Implemented {
		return "", fmt.Errorf("no engine image to build for %q: build one of %v", name, implementedNames())
	}
	text, _ := dockerfiles.ReadFile("dockerfiles/" + spec.Name + ".Dockerfile") // every implemented engine has one (tested)
	dir, err := os.MkdirTemp("", "rea-engine-build")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	_ = os.WriteFile(filepath.Join(dir, "Dockerfile"), text, 0o644) // a file that was not written makes docker report the missing Dockerfile
	out, err := s.exec().Run(ctx, "docker", "build", "-t", spec.Image, dir)
	return string(out), err
}
