package directive

import "fmt"

// RefusalOwnerAGY hands a ticket a Claude session refused to Antigravity (AGY); RefusalOwnerNone keeps it with Claude.
const (
	RefusalOwnerAGY  = "agy"
	RefusalOwnerNone = "none"
)

// ReaOwnerLabel marks a ticket AGY took over from a Claude session that refused it.
const ReaOwnerLabel = "owner:agy"

// Rea says which tickets are reverse-engineering (REA) work and who owns one a Claude session refused.
// A ticket is REA work when it carries one of Labels or its title starts with one of TitlePrefixes.
type Rea struct {
	Labels        []string `toml:"labels"`
	TitlePrefixes []string `toml:"title_prefixes"`
	RefusalOwner  string   `toml:"refusal_owner"`
	RefusalModel  string   `toml:"refusal_model"`
	// Engine is the reverse-engineering tool to run over a target: auto (the best usable open-source one), none,
	// or a name of the engine catalog. Sandbox is where it runs: container (the default) or host.
	Engine  string `toml:"engine"`
	Sandbox string `toml:"sandbox"`
}

// defaultRea is the REA directive of a project that has none: AGY takes over a refused REA ticket.
func defaultRea() Rea {
	return Rea{
		Labels:        []string{"rea"},
		TitlePrefixes: []string{"[REA]"},
		RefusalOwner:  RefusalOwnerAGY,
		Engine:        "auto",
		Sandbox:       "container",
	}
}

// validate reports the first REA setting that cannot be used.
func (r Rea) validate() error {
	if !oneOf(r.RefusalOwner, RefusalOwnerAGY, RefusalOwnerNone) {
		return fmt.Errorf("rea.refusal_owner must be %s or %s", RefusalOwnerAGY, RefusalOwnerNone)
	}
	if !oneOf(r.Engine, "auto", "none", "rizin", "radare2", "objdump", "ghidra") { // the names of pkg/rea/engine's catalog
		return fmt.Errorf("rea.engine must be auto, none, rizin, radare2, objdump or ghidra")
	}
	if !oneOf(r.Sandbox, "container", "host") {
		return fmt.Errorf("rea.sandbox must be container or host")
	}
	if r.RefusalOwner == RefusalOwnerAGY && len(r.Labels) == 0 && len(r.TitlePrefixes) == 0 {
		return fmt.Errorf("rea.labels or rea.title_prefixes needs an entry when rea.refusal_owner is %s", RefusalOwnerAGY)
	}
	return nil
}
