package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/rea"
)

// Engine names that are not tools.
const (
	EngineAuto = "auto"
	EngineNone = "none"
)

// Policy is what the user asked for: an engine (auto picks the best usable one, none disables engines), where it
// runs (container by default), and whether a binary may be opened on the host. Trusted is the explicit statement
// that the target is safe to open on the host.
type Policy struct {
	Engine  string `json:"engine"`
	Sandbox string `json:"sandbox"`
	Trusted bool   `json:"trusted"`
}

// Env is what the machine offers: whether Docker answers, whether an image is present and where a host binary is.
type Env struct {
	DockerUp func(ctx context.Context) bool
	HasImage func(ctx context.Context, image string) bool
	LookPath func(name string) (string, error)
}

// Choice is the engine and the mode it will run in. HostBin is the binary to run in host mode.
type Choice struct {
	Spec    Spec
	Mode    string
	HostBin string
}

// Decision is the outcome of Select: a choice, or no choice and why.
type Decision struct {
	Choice *Choice
	Note   string
}

// untrusted reports whether a target of this format may carry hostile content: everything but text the project
// can read for itself. A disassembler parsing a hostile binary has had memory-safety bugs, so those are opened only
// in the container unless the user says otherwise.
func untrusted(format string) bool {
	return format != coreclient.FormatSourceTree && format != coreclient.FormatJSBundle
}

func (p Policy) normalized() (Policy, error) {
	if p.Engine == "" {
		p.Engine = EngineAuto
	}
	if p.Sandbox == "" {
		p.Sandbox = ModeContainer
	}
	if p.Sandbox != ModeContainer && p.Sandbox != ModeHost {
		return p, fmt.Errorf("unknown sandbox %q: use %s or %s", p.Sandbox, ModeContainer, ModeHost)
	}
	if _, known := Lookup(p.Engine); !known && p.Engine != EngineAuto && p.Engine != EngineNone {
		return p, fmt.Errorf("unknown engine %q: use auto, none or one of %s", p.Engine, strings.Join(names(), ", "))
	}
	return p, nil
}

func names() []string {
	var out []string
	for _, s := range Catalog() {
		out = append(out, s.Name)
	}
	return out
}

// ranked is the catalog ordered by capability, best first, keeping catalog order among equals.
func ranked() []Spec {
	specs := Catalog()
	sort.SliceStable(specs, func(i, j int) bool { return specs[i].Caps.Score() > specs[j].Caps.Score() })
	return specs
}

// usable reports whether spec can run in mode on this machine; when it cannot, the error says why and how to fix it.
func usable(ctx context.Context, spec Spec, mode string, env Env) (string, *UnusableError) {
	if mode == ModeContainer {
		switch {
		case !env.DockerUp(ctx):
			return "", &UnusableError{spec.Name, mode, "Docker does not answer", "start Docker"}
		case !env.HasImage(ctx, spec.Image):
			return "", &UnusableError{spec.Name, mode, "image " + spec.Image + " is not built", "dendrite retro engines build " + spec.Name}
		}
		return "", nil
	}
	for _, bin := range spec.HostBins {
		if _, err := env.LookPath(bin); err == nil {
			return bin, nil
		}
	}
	return "", &UnusableError{spec.Name, mode, "none of " + strings.Join(spec.HostBins, ", ") + " is on PATH", "install " + spec.Name}
}

// Select chooses the engine for a target of this format. In auto mode it takes the first usable engine by
// capability (all of them open-source) and, when none is usable, answers no engine and why instead of failing. An
// explicit engine that cannot run is an error that names the fix. A binary is never opened on the host unless the
// policy says Trusted.
func Select(ctx context.Context, p Policy, format string, env Env) (Decision, error) {
	p, err := p.normalized()
	if err != nil {
		return Decision{}, err
	}
	if p.Engine == EngineNone {
		return Decision{Note: "engines are disabled (engine = none)"}, nil
	}
	if p.Sandbox == ModeHost && untrusted(format) && !p.Trusted {
		return Decision{}, fmt.Errorf("%w: a %s target is not opened on the host; use the container or pass --trusted", rea.ErrUntrustedOnHost, format)
	}
	if p.Engine != EngineAuto {
		return selectNamed(ctx, p, env)
	}
	var reasons []string
	for _, spec := range ranked() {
		if !spec.Implemented {
			continue
		}
		bin, problem := usable(ctx, spec, p.Sandbox, env)
		if problem == nil {
			return Decision{Choice: &Choice{Spec: spec, Mode: p.Sandbox, HostBin: bin}}, nil
		}
		reasons = append(reasons, problem.Engine+": "+problem.Reason+" ("+problem.Fix+")")
	}
	return Decision{Note: "no engine is usable in " + p.Sandbox + " mode: " + strings.Join(reasons, "; ")}, nil
}

// selectNamed handles an explicit engine: it must exist, be implemented and be usable.
func selectNamed(ctx context.Context, p Policy, env Env) (Decision, error) {
	spec, _ := Lookup(p.Engine)
	if !spec.Implemented {
		return Decision{}, fmt.Errorf("engine %s is planned but not implemented yet: use %s", spec.Name, strings.Join(implementedNames(), ", "))
	}
	bin, problem := usable(ctx, spec, p.Sandbox, env)
	if problem != nil {
		return Decision{}, problem
	}
	return Decision{Choice: &Choice{Spec: spec, Mode: p.Sandbox, HostBin: bin}}, nil
}

func implementedNames() []string {
	var out []string
	for _, s := range Catalog() {
		if s.Implemented {
			out = append(out, s.Name)
		}
	}
	return out
}
