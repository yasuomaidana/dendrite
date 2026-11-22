// Package engine runs open-source reverse-engineering tools (rizin, radare2, objdump) over a target to list its
// functions and read their disassembly. Tools run in a hardened container by default; the engine is chosen by a
// parameter that prefers open-source and available tools (see Select). The structure of a target comes from the
// dendrite-core sidecar, never from here.
package engine

import (
	"errors"
	"fmt"
	"time"
)

// Sandbox modes.
const (
	ModeContainer = "container"
	ModeHost      = "host"
)

// TargetPath is where the target is mounted, read-only, inside a container. Commands refer to the target by this
// fixed path, so no user-chosen text ever reaches a shell.
const TargetPath = "/target/input"

// Tool families: engines of a family take the same commands and answer in the same format.
const (
	FamilyR2      = "r2"
	FamilyObjdump = "objdump"
	FamilyGhidra  = "ghidra"
	// FamilyProprietary marks licensed tools Dendrite documents but does not run (see docs/features/rea-engines.md).
	FamilyProprietary = "proprietary"
)

const proprietaryNote = "proprietary tool, documented only (docs/features/rea-engines.md)"

// DocsPath is where the engines, the selection policy and the proprietary tools are documented.
const DocsPath = "docs/features/rea-engines.md"

// Function is one function an engine found. Pseudocode stays empty until a decompiler backend exists.
type Function struct {
	Name        string   `json:"name"`
	Address     string   `json:"address"`
	Size        uint64   `json:"size"`
	Cyclomatic  int      `json:"cyclomatic,omitempty"`
	Calls       []string `json:"calls,omitempty"`
	Disassembly string   `json:"disassembly,omitempty"`
	Pseudocode  string   `json:"pseudocode,omitempty"`
}

// Caps are what an engine can do.
type Caps struct {
	Functions   bool `json:"functions"`
	Disassembly bool `json:"disassembly"`
	Decompile   bool `json:"decompile"`
	CallGraph   bool `json:"call_graph"`
}

// Score ranks capabilities: more is better, decompiling most.
func (c Caps) Score() int {
	score := 0
	for _, w := range []struct {
		on     bool
		weight int
	}{{c.Decompile, 4}, {c.CallGraph, 2}, {c.Functions, 2}, {c.Disassembly, 1}} {
		if w.on {
			score += w.weight
		}
	}
	return score
}

// Spec describes one known engine. Implemented is false for an engine the catalog lists but Dendrite cannot run (a
// planned one, or a proprietary tool that is documented only).
type Spec struct {
	Name        string `json:"name"`
	License     string `json:"license"`
	OpenSource  bool   `json:"open_source"`
	Proprietary bool   `json:"proprietary,omitempty"`
	Implemented bool   `json:"implemented"`
	// Note says why an engine that is not implemented cannot be run, and where to read more.
	Note   string `json:"note,omitempty"`
	Caps   Caps   `json:"caps"`
	Family string `json:"family"`
	Image  string `json:"image"`
	// HostBins are the binaries host mode looks for, first match wins.
	HostBins []string `json:"host_bins"`
	// MemoryMB and Timeout override the sandbox defaults for an engine that needs more (zero keeps the defaults).
	MemoryMB int           `json:"memory_mb,omitempty"`
	Timeout  time.Duration `json:"timeout,omitempty"`
	// Env is set inside the container, for a tool that needs a writable home on a read-only root.
	Env map[string]string `json:"env,omitempty"`
}

// Catalog lists the known engines in preference order: open-source first, then by capability. Proprietary tools are
// listed so they can be configured and documented, but are never selected automatically.
func Catalog() []Spec {
	return []Spec{
		{Name: "rizin", License: "LGPL-3.0", OpenSource: true, Implemented: true, Family: FamilyR2, Image: "dendrite-rea/rizin:1", HostBins: []string{"rizin"},
			Caps: Caps{Functions: true, Disassembly: true, CallGraph: true}},
		{Name: "radare2", License: "LGPL-3.0", OpenSource: true, Implemented: true, Family: FamilyR2, Image: "dendrite-rea/radare2:1", HostBins: []string{"radare2", "r2"},
			Caps: Caps{Functions: true, Disassembly: true, CallGraph: true}},
		{Name: "objdump", License: "GPL-3.0-or-later", OpenSource: true, Implemented: true, Family: FamilyObjdump, Image: "dendrite-rea/objdump:1", HostBins: []string{"objdump", "llvm-objdump"},
			Caps: Caps{Functions: true, Disassembly: true}},
		{Name: "ghidra", License: "Apache-2.0", OpenSource: true, Implemented: true, Family: FamilyGhidra, Image: "dendrite-rea/ghidra:1", HostBins: []string{"analyzeHeadless"},
			Caps: Caps{Functions: true, Disassembly: true, Decompile: true, CallGraph: true}, MemoryMB: 4096, Timeout: 20 * time.Minute,
			Env: map[string]string{"HOME": "/tmp", "JAVA_TOOL_OPTIONS": "-Duser.home=/tmp"}},
		{Name: "hopper", Note: proprietaryNote, License: "proprietary (Cryptic Apps)", Proprietary: true, Family: FamilyProprietary, HostBins: []string{"hopper", "Hopper"},
			Caps: Caps{Functions: true, Disassembly: true, Decompile: true, CallGraph: true}},
		{Name: "ida", Note: proprietaryNote, License: "proprietary (Hex-Rays)", Proprietary: true, Family: FamilyProprietary, HostBins: []string{"idat64", "idat"},
			Caps: Caps{Functions: true, Disassembly: true, Decompile: true, CallGraph: true}},
		{Name: "binaryninja", Note: proprietaryNote, License: "proprietary (Vector 35)", Proprietary: true, Family: FamilyProprietary, HostBins: []string{"binaryninja"},
			Caps: Caps{Functions: true, Disassembly: true, Decompile: true, CallGraph: true}},
	}
}

// Lookup finds an engine of the catalog by name.
func Lookup(name string) (Spec, bool) {
	for _, s := range Catalog() {
		if s.Name == name {
			return s, true
		}
	}
	return Spec{}, false
}

// ErrUnavailable is what an engine that cannot run answers; errors.Is(err, ErrUnavailable) holds for an
// *UnusableError.
var ErrUnavailable = errors.New("engine unavailable")

// UnusableError says why an engine cannot run in a mode and what to do about it.
type UnusableError struct {
	Engine, Mode, Reason, Fix string
}

func (e *UnusableError) Error() string {
	return fmt.Sprintf("%s is not usable in %s mode: %s (%s)", e.Engine, e.Mode, e.Reason, e.Fix)
}

// Is makes an UnusableError match ErrUnavailable.
func (e *UnusableError) Is(target error) bool { return target == ErrUnavailable }
