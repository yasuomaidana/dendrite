package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"Dendrite/pkg/rea"
)

// machine is a description of what a computer offers: whether Docker is up, which images are built and which binaries
// are on PATH. It builds the Env a test passes to Select.
type machine struct {
	docker bool
	images []string
	bins   []string
}

func (m machine) env() Env {
	has := func(list []string, item string) bool {
		for _, l := range list {
			if l == item {
				return true
			}
		}
		return false
	}
	return Env{
		DockerUp: func(context.Context) bool { return m.docker },
		HasImage: func(_ context.Context, image string) bool { return has(m.images, image) },
		LookPath: func(name string) (string, error) {
			if has(m.bins, name) {
				return "/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
	}
}

func choose(t *testing.T, m machine, p Policy, format string) Decision {
	t.Helper()
	d, err := Select(context.Background(), p, format, m.env())
	if err != nil {
		t.Fatalf("%+v on %+v: %v", p, m, err)
	}
	return d
}

func TestCatalogSeparatesOpenSourceFromProprietaryTools(t *testing.T) {
	for _, s := range Catalog() {
		switch {
		case s.Proprietary:
			if s.OpenSource || s.Implemented || s.Family != FamilyProprietary || s.License == "" || len(s.HostBins) == 0 {
				t.Errorf("a proprietary tool is documented, not run: %+v", s)
			}
		default:
			if !s.OpenSource || !s.Implemented || s.License == "" || s.Image == "" || len(s.HostBins) == 0 {
				t.Errorf("an open-source engine is runnable: %+v", s)
			}
		}
	}
	if s, ok := Lookup("radare2"); !ok || s.Family != FamilyR2 {
		t.Errorf("%+v %t", s, ok)
	}
	for _, name := range []string{"hopper", "ida", "binaryninja"} {
		if s, ok := Lookup(name); !ok || !s.Proprietary {
			t.Errorf("%s: %+v %t", name, s, ok)
		}
	}
}

func TestEnginesAreRankedByCapability(t *testing.T) {
	var implemented []string
	for _, s := range ranked() {
		if s.Implemented {
			implemented = append(implemented, s.Name)
		}
	}
	if strings.Join(implemented, ",") != "ghidra,rizin,radare2,objdump" {
		t.Errorf("the best engine first, got %v", implemented)
	}
	if (Caps{}).Score() != 0 || (Caps{Decompile: true, CallGraph: true, Functions: true, Disassembly: true}).Score() != 9 {
		t.Error("capability scores add up")
	}
}

func TestPolicyDefaultsAndRefusals(t *testing.T) {
	m := machine{docker: true, images: []string{"dendrite-rea/rizin:1"}}
	d := choose(t, m, Policy{}, "elf")
	if d.Choice == nil || d.Choice.Spec.Name != "rizin" || d.Choice.Mode != ModeContainer {
		t.Errorf("the defaults are auto and container: %+v", d)
	}
	for _, p := range []Policy{{Sandbox: "vm"}, {Engine: "emacs"}, {Engine: "ida"}} {
		if _, err := Select(context.Background(), p, "elf", m.env()); err == nil {
			t.Errorf("%+v must be refused", p)
		}
	}
	if d := choose(t, m, Policy{Engine: EngineNone}, "elf"); d.Choice != nil || !strings.Contains(d.Note, "disabled") {
		t.Errorf("engines off: %+v", d)
	}
}

func TestAnUntrustedTargetIsNeverOpenedOnTheHostUnlessTrusted(t *testing.T) {
	m := machine{bins: []string{"rizin"}}
	for _, format := range []string{"pe", "elf", "macho", "fat-macho", "zip", "asar", "unknown"} {
		_, err := Select(context.Background(), Policy{Sandbox: ModeHost}, format, m.env())
		if !errors.Is(err, rea.ErrUntrustedOnHost) || !strings.Contains(err.Error(), "--trusted") {
			t.Errorf("%s on the host: %v", format, err)
		}
	}
	if d := choose(t, m, Policy{Sandbox: ModeHost, Trusted: true}, "elf"); d.Choice == nil || d.Choice.Mode != ModeHost || d.Choice.HostBin != "rizin" {
		t.Errorf("a trusted binary may run on the host: %+v", d)
	}
	for _, format := range []string{"source-tree", "js-bundle"} {
		if d := choose(t, m, Policy{Sandbox: ModeHost}, format); d.Choice == nil {
			t.Errorf("text may be read on the host: %s %+v", format, d)
		}
	}
}

func TestAutoPrefersTheBestUsableOpenSourceEngine(t *testing.T) {
	for name, tc := range map[string]struct {
		m    machine
		p    Policy
		want string
		bin  string
	}{
		"rizin when its image is built":      {machine{docker: true, images: []string{"dendrite-rea/rizin:1", "dendrite-rea/objdump:1"}}, Policy{}, "rizin", ""},
		"radare2 when only its image is":     {machine{docker: true, images: []string{"dendrite-rea/radare2:1"}}, Policy{}, "radare2", ""},
		"objdump as the lightweight default": {machine{docker: true, images: []string{"dendrite-rea/objdump:1"}}, Policy{}, "objdump", ""},
		"llvm-objdump on the host":           {machine{bins: []string{"llvm-objdump"}}, Policy{Sandbox: ModeHost, Trusted: true}, "objdump", "llvm-objdump"},
		"r2 on the host":                     {machine{bins: []string{"r2", "objdump"}}, Policy{Sandbox: ModeHost, Trusted: true}, "radare2", "r2"},
	} {
		d := choose(t, tc.m, tc.p, "elf")
		if d.Choice == nil || d.Choice.Spec.Name != tc.want || d.Choice.HostBin != tc.bin {
			t.Errorf("%s: %+v", name, d)
		}
	}
}

func TestAutoWithNothingUsableSaysWhyForEachEngine(t *testing.T) {
	down := choose(t, machine{}, Policy{}, "elf")
	if down.Choice != nil || !strings.Contains(down.Note, "Docker does not answer") || !strings.Contains(down.Note, "rizin") || !strings.Contains(down.Note, "objdump") || !strings.Contains(down.Note, "ghidra") || strings.Contains(down.Note, "hopper") || strings.Contains(down.Note, "ida:") {
		t.Errorf("Docker down: %+v", down)
	}
	noImages := choose(t, machine{docker: true}, Policy{}, "elf")
	if noImages.Choice != nil || !strings.Contains(noImages.Note, "dendrite retro engines build rizin") {
		t.Errorf("no images: %+v", noImages)
	}
	noBins := choose(t, machine{}, Policy{Sandbox: ModeHost, Trusted: true}, "elf")
	if noBins.Choice != nil || !strings.Contains(noBins.Note, "none of rizin is on PATH") {
		t.Errorf("no binaries: %+v", noBins)
	}
}

func TestAnExplicitEngineMustBeUsableAndNeverFallsBack(t *testing.T) {
	m := machine{docker: true, images: []string{"dendrite-rea/objdump:1"}}
	_, err := Select(context.Background(), Policy{Engine: "rizin"}, "elf", m.env())
	var unusable *UnusableError
	if !errors.As(err, &unusable) || !errors.Is(err, ErrUnavailable) || unusable.Fix != "dendrite retro engines build rizin" ||
		!strings.Contains(err.Error(), "rizin is not usable in container mode") {
		t.Errorf("an engine without an image: %v", err)
	}
	for _, name := range []string{"hopper", "ida", "binaryninja"} {
		_, err := Select(context.Background(), Policy{Engine: name}, "elf", m.env())
		if err == nil || !strings.Contains(err.Error(), "proprietary tool") || !strings.Contains(err.Error(), DocsPath) || !strings.Contains(err.Error(), "rizin, radare2, objdump, ghidra") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if d := choose(t, m, Policy{Engine: "objdump"}, "elf"); d.Choice == nil || d.Choice.Spec.Name != "objdump" {
		t.Errorf("an explicit engine that is usable: %+v", d)
	}
}

func TestUnusableErrorFormatsTheFix(t *testing.T) {
	e := &UnusableError{Engine: "x", Mode: "host", Reason: "r", Fix: "f"}
	if got := fmt.Sprint(e); got != "x is not usable in host mode: r (f)" {
		t.Errorf("%q", got)
	}
}

func TestAutoPrefersGhidraWhenItsImageIsBuiltAndNeverAProprietaryTool(t *testing.T) {
	m := machine{docker: true, images: []string{"dendrite-rea/ghidra:1", "dendrite-rea/rizin:1"}, bins: []string{"hopper", "idat64", "binaryninja"}}
	if d := choose(t, m, Policy{}, "elf"); d.Choice == nil || d.Choice.Spec.Name != "ghidra" || d.Choice.Mode != ModeContainer {
		t.Errorf("the best usable engine: %+v", d)
	}
	host := machine{bins: []string{"hopper", "idat64", "binaryninja"}}
	if d := choose(t, host, Policy{Sandbox: ModeHost, Trusted: true}, "elf"); d.Choice != nil {
		t.Errorf("a proprietary tool on PATH is not used by auto: %+v", d.Choice)
	}
}
