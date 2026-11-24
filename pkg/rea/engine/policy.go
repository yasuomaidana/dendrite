package engine

import (
	"os"

	"Dendrite/pkg/directive"
)

// Environment variables that set the engine and the sandbox when no flag does.
const (
	EnvEngine  = "DENDRITE_REA_ENGINE"
	EnvSandbox = "DENDRITE_REA_SANDBOX"
)

// ResolvePolicy fills what a caller left empty: the flag (already in p) wins, then the environment, then the
// project's [rea] directive. Trusted is only ever what the caller says.
func ResolvePolicy(rule directive.Rea, p Policy) Policy {
	if p.Engine == "" {
		p.Engine = firstSet(os.Getenv(EnvEngine), rule.Engine)
	}
	if p.Sandbox == "" {
		p.Sandbox = firstSet(os.Getenv(EnvSandbox), rule.Sandbox)
	}
	return p
}

func firstSet(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
