package pipeline

import (
	"fmt"
	"strings"
)

// toolDoc is how the generated agent documents describe a toolchain.
type toolDoc struct{ label, rule string }

var toolDocs = map[string]toolDoc{
	"go":    {"Go", "`go` (`go test -race ./...`, `golangci-lint run`)"},
	"cargo": {"Rust", "`cargo` (`cargo test`, `cargo clippy`, `cargo fmt --check`)"},
	"bun":   {"JavaScript / TypeScript", "`bun` (`bun install --frozen-lockfile`, `bun run test`, `bun run build`); legacy `npm` and `yarn` are banned"},
	"uv":    {"Python", "`uv` (`uv run pytest`, `uvx ruff check .`); legacy `pip install` is banned"},
}

// ToolchainBullets lists the toolchains a project uses, one bullet each, in the order given.
func ToolchainBullets(names []string) string {
	var b strings.Builder
	for _, n := range names {
		if d, ok := toolDocs[n]; ok {
			fmt.Fprintf(&b, "- **%s**: %s\n", d.label, d.rule)
		}
	}
	if b.Len() == 0 {
		return "- No build toolchain detected yet.\n"
	}
	return b.String()
}

// ToolchainSummary is the one-line form: "Go (`go`), Rust (`cargo`)".
func ToolchainSummary(names []string) string {
	var parts []string
	for _, n := range names {
		if d, ok := toolDocs[n]; ok {
			parts = append(parts, fmt.Sprintf("%s (`%s`)", d.label, n))
		}
	}
	if len(parts) == 0 {
		return "none detected"
	}
	return strings.Join(parts, ", ")
}
