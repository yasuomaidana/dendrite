package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"Dendrite/pkg/dotenv"
	"Dendrite/pkg/projectid"
)

func renderCopilotInstructions(fp ProjectFingerprint) string {
	var sb strings.Builder
	sb.WriteString("# GitHub Copilot Custom Instructions\n\n")
	sb.WriteString("> Authoritative developer guidelines and architectural constraints for AI code generation.\n\n")
	sb.WriteString("## 1. Zero Code Without Plane Ticket Mandate\n")
	sb.WriteString("- No agent, AI assistant, or engineer is permitted to author code or modify tests without an active, authoritative ticket in Plane v2 (`http://localhost:8090`).\n")
	sb.WriteString("- Every commit must reference an active Plane ticket (e.g., `[DEN-XXX]`).\n\n")

	sb.WriteString("## 2. Test-Driven Development (TDD) & Anti-Mock Mandate\n")
	sb.WriteString("- Author tests before or alongside code implementation.\n")
	sb.WriteString("- **Zero Mock Policy**: Do NOT use mocking frameworks, fake stubs, or synthetic dummies for persistence or protocols.\n")
	sb.WriteString("- All persistent database layers (Postgres, Redis, MinIO) must use **Testcontainers** for real integration tests.\n")
	sb.WriteString("- Maintain minimum statement coverage: **>= 85.0%** across all new packages.\n\n")

	sb.WriteString("## 3. Approved & Enforced Toolchains (Go, Cargo, Bun, uv)\n")
	sb.WriteString("All tooling and dependency operations must use project-approved toolchains:\n")
	sb.WriteString("- **Go**: `go` (`go test -v -race -cover ./pkg/...`, `golangci-lint`)\n")
	sb.WriteString("- **Rust**: `cargo` (`cargo test --verbose`, `cargo build --release`, `cargo clippy`)\n")
	sb.WriteString("- **JavaScript / TypeScript**: `bun` (`bun test`, `bun run build`, `bun install --frozen-lockfile`) - legacy `npm` and `yarn` are strictly banned\n")
	sb.WriteString("- **Python**: `uv` (`uv run pytest`, `uv pip list`, `uv venv`) - legacy `pip install` is strictly banned\n\n")

	sb.WriteString("## 4. Technology Stack & Coding Standards\n")
	sb.WriteString(fmt.Sprintf("- **Primary Stack**: %s\n", strings.ToUpper(string(fp.PrimaryStack))))
	if fp.GoVersion != "" {
		sb.WriteString(fmt.Sprintf("- **Go Toolchain**: %s\n", fp.GoVersion))
	}
	if len(fp.Toolchains) > 0 {
		sb.WriteString(fmt.Sprintf("- **Active Toolchains**: %s\n", strings.Join(fp.Toolchains, ", ")))
	}
	if len(fp.Linters) > 0 {
		sb.WriteString(fmt.Sprintf("- **Enforced Linters**: %s\n", strings.Join(fp.Linters, ", ")))
	}
	sb.WriteString("- **Complexity Limits**: Cyclomatic Complexity <= 15 per function, Cognitive Complexity <= 20, max function length <= 80 lines.\n\n")

	sb.WriteString("## 5. Model Precedence & Zero-Cost Delegation\n")
	sb.WriteString("- Commit messages and repetitive test digestions must be delegated to local zero-cost models (`qwen3.8:27b` / Ollama Metal) to conserve API tokens.\n")
	sb.WriteString("- Cloud models (Gemini Pro, Copilot Sonnet) are reserved for macro architecture, complex decomposition, and review gates.\n")

	return sb.String()
}

// ManagedHeader marks a file Dendrite generates and may rewrite. A file without it was written by a person
// and is left alone.
const ManagedHeader = "# managed by dendrite — edit .github/dendrite/directives.toml"

func renderCIWorkflow(fp ProjectFingerprint) string {
	var sb strings.Builder
	sb.WriteString(ManagedHeader + "\n")
	sb.WriteString("name: Dendrite CI/CD\n\n")
	sb.WriteString("on:\n")
	sb.WriteString("  push:\n")
	sb.WriteString("    branches: [ develop, main ]\n")
	sb.WriteString("  pull_request:\n")
	sb.WriteString("    branches: [ develop, main ]\n\n")
	sb.WriteString("jobs:\n")
	sb.WriteString("  verify:\n")
	sb.WriteString("    name: Test & Coverage Gate (>= 85%)\n")
	sb.WriteString("    runs-on: ubuntu-latest\n")
	sb.WriteString("    steps:\n")
	sb.WriteString("      - name: Checkout Code\n")
	sb.WriteString("        uses: actions/checkout@v4\n\n")

	// 1. Go toolchain (if Go is detected or is primary)
	if fp.HasGoMod || fp.PrimaryStack == StackGo {
		goVer := fp.GoVersion
		if goVer == "" {
			goVer = "1.25"
		}
		sb.WriteString("      - name: Set up Go\n")
		sb.WriteString("        uses: actions/setup-go@v5\n")
		sb.WriteString(fmt.Sprintf("        with:\n          go-version: '%s'\n          cache: true\n\n", goVer))
		sb.WriteString("      - name: Run Tests with Coverage\n")
		sb.WriteString("        run: |\n          go test -v -race -coverprofile=coverage.out -covermode=atomic ./pkg/...\n")
		sb.WriteString("      - name: Verify Coverage Threshold (>= 85%)\n")
		sb.WriteString("        run: |\n          total=$(go tool cover -func=coverage.out | grep 'total:' | awk '{print $3}' | sed 's/%//')\n          echo \"Total Statement Coverage: ${total}%\"\n          awk -v cov=\"$total\" 'BEGIN { if (cov < 85.0) { print \"[-] Statement coverage \" cov \"% is below required 85.0%\"; exit 1 } else { print \"[\u2713] Coverage gate passed: \" cov \"%\"; exit 0 } }'\n\n")
	}

	// 2. Rust & Cargo toolchain
	if fp.HasCargoToml || fp.HasCargo || fp.PrimaryStack == StackRust {
		sb.WriteString("      - name: Set up Rust (cargo)\n")
		sb.WriteString("        uses: dtolnay/rust-toolchain@stable\n\n")
		sb.WriteString("      - name: Run Cargo Tests\n")
		sb.WriteString("        run: cargo test --verbose\n\n")
	}

	// 3. JS/TS & Bun toolchain
	if fp.HasPackageJSON || fp.HasBun || fp.PrimaryStack == StackJavaScript || fp.PrimaryStack == StackTypeScript {
		sb.WriteString("      - name: Set up Bun\n")
		sb.WriteString("        uses: oven-sh/setup-bun@v2\n")
		sb.WriteString("        with:\n          bun-version: latest\n\n")
		sb.WriteString("      - name: Run Bun Tests & Build\n")
		sb.WriteString("        run: |\n          if [ -d \"website\" ]; then\n            cd website && bun install --frozen-lockfile && bun run build\n          else\n            bun test\n          fi\n\n")
	}

	// 4. Python & uv toolchain
	if fp.HasPyProject || fp.HasUV || fp.PrimaryStack == StackPython {
		sb.WriteString("      - name: Set up uv (Python)\n")
		sb.WriteString("        uses: astral-sh/setup-uv@v5\n")
		sb.WriteString("        with:\n          enable-cache: true\n\n")
		sb.WriteString("      - name: Verify Python Environment with uv\n")
		sb.WriteString("        run: |\n          uv --version\n          if [ -f \"scripts/pyproject.toml\" ]; then\n            cd scripts && uv pip list || true\n          fi\n")
	}

	return sb.String()
}

func renderAgentRules(fp ProjectFingerprint) string {
	var sb strings.Builder
	sb.WriteString("# Autonomous Agent Operating Rules\n\n")
	sb.WriteString("## Authoritative Project Management & Plane Ownership\n")
	sb.WriteString("- Plane v2 (`http://localhost:8090`) is the authoritative owner of ticket state.\n")
	sb.WriteString("- NEVER use `DEV_STATUS.md` to decide what to work on; query Plane via `dendrite chronos next` or `dendrite_ticket_smart_select`.\n")
	sb.WriteString("- Completed features must transition to `To Verify` (intermediate verification state) for human sign-off, never directly to `Done`.\n\n")

	sb.WriteString("## Architecture & CI/CD Tooling Defaults\n")
	sb.WriteString("- Default CI/CD & Assistant Tool: **GitHub Copilot** (`.github/copilot-instructions.md`).\n")
	sb.WriteString(fmt.Sprintf("- Primary Stack: `%s`\n", fp.PrimaryStack))
	sb.WriteString("- Standardized Toolsets:\n")
	sb.WriteString("  - **Go**: `go test -v -race`, `golangci-lint`\n")
	sb.WriteString("  - **Rust**: `cargo test`, `cargo build`, `cargo clippy`\n")
	sb.WriteString("  - **JavaScript / TypeScript**: `bun test`, `bun run build` (zero npm)\n")
	sb.WriteString("  - **Python**: `uv run`, `uv pip` (zero legacy pip)\n")
	sb.WriteString("- Statement coverage threshold: **>= 85.0%**.\n")
	sb.WriteString("- Anti-mock policy: genuine structs and Testcontainers only.\n\n")

	sb.WriteString("## Dendrite-First Workflow (every model and session)\n")
	writeNumberedRules(&sb)

	return sb.String()
}

func renderAgentsOverview(fp ProjectFingerprint) string {
	var sb strings.Builder
	sb.WriteString("# Project Agents & Environment Orchestration\n\n")
	sb.WriteString("This repository is orchestrated with autonomous multi-agent loops and governed by Dendrite.\n\n")
	sb.WriteString("## Standard Tooling\n")
	sb.WriteString("- **Default CI/CD Assistant**: GitHub Copilot (`.github/copilot-instructions.md`)\n")
	sb.WriteString(fmt.Sprintf("- **Runtime Stack**: %s\n", strings.ToUpper(string(fp.PrimaryStack))))
	sb.WriteString("- **Enforced Toolchains**: Go (`go`), Rust (`cargo`), Bun (`bun`), uv (`uv`)\n")
	sb.WriteString("- **Authoritative Issue Tracker**: Plane v2 (`http://localhost:8090`)\n")
	sb.WriteString("- **Token Governor**: LiteLLM Gateway (`http://localhost:4000`)\n\n")
	sb.WriteString("## Preparation & Sync\n")
	sb.WriteString("To synchronize agent rules, skills, and enforce CI/CD tooling defaults, execute:\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite soma prepare   # alias: dendrite env prepare\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## Dendrite-First Workflow (all models: Claude, Antigravity/AGY, Gemini, Copilot, humans)\n")
	writeNumberedRules(&sb)
	sb.WriteString(fmt.Sprintf("\nSee the `%s` skill for commands and cleanup rules.\n\n", DendriteWorkflowSkill))

	sb.WriteString("## Commit & VCS Guidance\n")
	sb.WriteString("- `dendrite quantal` (alias `commit`) pushes the new commit to the configured remote (origin preferred) after every successful commit; use `--no-push` or `DENDRITE_COMMIT_PUSH=false` to opt out; it never force-pushes and refuses to push main/master.\n")
	sb.WriteString("- `dendrite quantal --synapse[=<id>]` commits the current project, then walks its synapse peers: dirty peers are committed through the same AI-assisted engine after confirmation (`--yes` skips the prompt), and clean peers ahead of their upstream are pushed.\n")
	sb.WriteString("- `dendrite sync` pushes unpushed commits and refreshes the Plane board (canonical states + DEV_STATUS when it changed) for the current project and every synapse peer, then records synapse health (`active`/`broken`). `dendrite sync --all --watch` keeps every synapse current (`DENDRITE_SYNC_INTERVAL`, default 10m).\n")
	sb.WriteString("- Plane workflow states are canonical across projects (`plane.CanonicalStates`); fix drift with `dendrite chronos states reconcile --all`.\n\n")

	sb.WriteString("## Session Start & Handoff\n")
	sb.WriteString("Every new session (Claude, AGY, or human) starts with:\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite session resume\n")
	sb.WriteString("```\n")
	sb.WriteString("Orchestrators run `dendrite session checkpoint` after each finished ticket and before long waits:\n")
	sb.WriteString("```bash\n")
	sb.WriteString("dendrite session checkpoint [--worktree dir] [--decision \"...\"]... [--next \"DEN-XXX\"]... [--note \"...\"]\n")
	sb.WriteString("```\n")

	return sb.String()
}

func writeOrUpdateFile(path, content string, dryRun bool) (bool, bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil {
		if string(existing) == content {
			return false, false, nil // No change
		}
		if !dryRun {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return false, false, err
			}
		}
		return false, true, nil // Updated
	}

	if os.IsNotExist(err) {
		if !dryRun {
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return false, false, err
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return false, false, err
			}
		}
		return true, false, nil // Created
	}

	return false, false, err
}

// writeManagedFile is writeOrUpdateFile for a file a person may own: it creates the file when missing and
// rewrites it only while it carries ManagedHeader. kept is true when an existing file without the header was
// left untouched.
func writeManagedFile(path, content string, dryRun bool) (created, updated, kept bool, err error) {
	if existing, readErr := os.ReadFile(path); readErr == nil && !strings.HasPrefix(string(existing), ManagedHeader) {
		return false, false, true, nil
	}
	created, updated, err = writeOrUpdateFile(path, content, dryRun)
	return created, updated, false, err
}

// RagEnvFile, under the Dendrite home, holds the DENDRITE_RAG_* variables the sidecar
// launcher passes to dendrite-rag (pkg/sidecar).
const RagEnvFile = "rag.env"

// ensureRagEnv adds DENDRITE_RAG_PG_URL, the shared-tier database of the control stack, to
// $DENDRITE_HOME/rag.env when it is missing; an existing value is never changed. The host
// port follows POSTGRES_PORT (environment, then the project's .env), default 5432.
func ensureRagEnv(projectRoot string, dryRun bool) (created, updated bool, err error) {
	home, err := projectid.HomeDir()
	if err != nil {
		return false, false, err
	}
	path := filepath.Join(home, RagEnvFile)
	data, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, false, readErr
	}
	if dotEnvValue(string(data), "DENDRITE_RAG_PG_URL") != "" {
		return false, false, nil
	}
	port := strings.TrimSpace(os.Getenv("POSTGRES_PORT"))
	if port == "" {
		port = dotenv.Lookup(projectRoot, "POSTGRES_PORT") // .env.local, then the global file, then the committed .env
	}
	if port == "" {
		port = "5432"
	}
	created, updated = readErr != nil, readErr == nil
	if dryRun {
		return created, updated, nil
	}
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += fmt.Sprintf("DENDRITE_RAG_PG_URL=postgresql://dendrite_rag:dendrite_rag@localhost:%s/dendrite_rag\n", port)
	if err := os.MkdirAll(home, 0o755); err != nil {
		return false, false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return false, false, err
	}
	return created, updated, nil
}

// dotEnvValue returns key's value in KEY=VALUE text, unquoted; "" when absent.
func dotEnvValue(text, key string) string {
	for _, line := range strings.Split(text, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), key+"="); ok {
			return strings.Trim(strings.TrimSpace(v), "\"'")
		}
	}
	return ""
}
