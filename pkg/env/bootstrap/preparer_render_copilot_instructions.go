package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/dotenv"
	"Dendrite/pkg/env/pipeline"
	"Dendrite/pkg/projectid"
)

func renderCopilotInstructions(fp ProjectFingerprint, set directive.Set) string {
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
	sb.WriteString(fmt.Sprintf("- Maintain minimum test coverage: **>= %s%%** (`coverage.min_percent` in `.github/dendrite/directives.toml`).\n\n", strconv.FormatFloat(set.Coverage.MinPercent, 'f', -1, 64)))

	sb.WriteString("## 3. Approved & Enforced Toolchains\n")
	sb.WriteString("This project uses these toolchains; all tooling and dependency operations go through them:\n")
	sb.WriteString(pipeline.ToolchainBullets(fp.Toolchains))
	sb.WriteString("\n")

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
	sb.WriteString(fmt.Sprintf("- **Complexity Limits**: cyclomatic complexity <= %d and cognitive complexity <= %d per function, at most %d production lines per file (`.github/dendrite/directives.toml`).\n\n", set.Complexity.MaxCyclomatic, set.Complexity.MaxCognitive, set.Loc.MaxProductionLines))

	sb.WriteString("## 5. Model Precedence & Zero-Cost Delegation\n")
	sb.WriteString("- Commit messages and repetitive test digestions must be delegated to local zero-cost models (`qwen3.8:27b` / Ollama Metal) to conserve API tokens.\n")
	sb.WriteString("- Cloud models (Gemini Pro, Copilot Sonnet) are reserved for macro architecture, complex decomposition, and review gates.\n")

	return sb.String()
}

// ManagedHeader marks a file Dendrite generates and may rewrite. A file without it was written by a person
// and is left alone.
const ManagedHeader = pipeline.ManagedHeader

func renderAgentRules(fp ProjectFingerprint, set directive.Set) string {
	var sb strings.Builder
	sb.WriteString("# Autonomous Agent Operating Rules\n\n")
	sb.WriteString("## Authoritative Project Management & Plane Ownership\n")
	sb.WriteString("- Plane v2 (`http://localhost:8090`) is the authoritative owner of ticket state.\n")
	sb.WriteString("- NEVER use `DEV_STATUS.md` to decide what to work on; query Plane via `dendrite chronos next` or `dendrite_ticket_smart_select`.\n")
	sb.WriteString("- Completed features must transition to `To Verify` (intermediate verification state) for human sign-off, never directly to `Done`.\n\n")

	sb.WriteString("## Architecture & CI/CD Tooling Defaults\n")
	sb.WriteString("- Default CI/CD & Assistant Tool: **GitHub Copilot** (`.github/copilot-instructions.md`).\n")
	sb.WriteString(fmt.Sprintf("- Primary Stack: `%s`\n", fp.PrimaryStack))
	sb.WriteString("- Toolchains of this project:\n")
	sb.WriteString(pipeline.ToolchainBullets(fp.Toolchains))
	sb.WriteString(fmt.Sprintf("- Test coverage threshold: **>= %s%%**; complexity per function: cyclomatic <= %d, cognitive <= %d.\n", strconv.FormatFloat(set.Coverage.MinPercent, 'f', -1, 64), set.Complexity.MaxCyclomatic, set.Complexity.MaxCognitive))
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
	sb.WriteString(fmt.Sprintf("- **Enforced Toolchains**: %s\n", pipeline.ToolchainSummary(fp.Toolchains)))
	sb.WriteString("- **Authoritative Issue Tracker**: Plane v2 (`http://localhost:8090`)\n")
	sb.WriteString("- **Token Governor**: LiteLLM Gateway (`http://localhost:4000`)\n\n")
	sb.WriteString("## Specs & Design Artifacts\n")
	sb.WriteString("Committed specs and EGM (`/egm:*`) design documents live under `docs/specs/`; per-ticket micro-plans live on the Plane ticket (`dendrite chronos plan push`), never under `.dendrite/`.\n\n")
	sb.WriteString("EGM artifact directory: docs/specs\n\n")
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
