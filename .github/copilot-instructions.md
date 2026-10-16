# GitHub Copilot Custom Instructions

> Authoritative developer guidelines and architectural constraints for AI code generation.

## 1. Zero Code Without Plane Ticket Mandate
- No agent, AI assistant, or engineer is permitted to author code or modify tests without an active, authoritative ticket in Plane v2 (`http://localhost:8090`).
- Every commit must reference an active Plane ticket (e.g., `[DEN-XXX]`).

## 2. Test-Driven Development (TDD) & Anti-Mock Mandate
- Author tests before or alongside code implementation.
- **Zero Mock Policy**: Do NOT use mocking frameworks, fake stubs, or synthetic dummies for persistence or protocols.
- All persistent database layers (Postgres, Redis, MinIO) must use **Testcontainers** for real integration tests.
- Maintain minimum statement coverage: **>= 85.0%** across all new packages.

## 3. Approved & Enforced Toolchains (Go, Cargo, Bun, uv)
All tooling and dependency operations must use project-approved toolchains:
- **Go**: `go` (`go test -v -race -cover ./pkg/...`, `golangci-lint`)
- **Rust**: `cargo` (`cargo test --verbose`, `cargo build --release`, `cargo clippy`)
- **JavaScript / TypeScript**: `bun` (`bun test`, `bun run build`, `bun install --frozen-lockfile`) - legacy `npm` and `yarn` are strictly banned
- **Python**: `uv` (`uv run pytest`, `uv pip list`, `uv venv`) - legacy `pip install` is strictly banned

## 4. Technology Stack & Coding Standards
- **Primary Stack**: GO
- **Go Toolchain**: 1.25
- **Active Toolchains**: go, bun, uv
- **Enforced Linters**: eslint, prettier, ruff, flake8
- **Complexity Limits**: Cyclomatic Complexity <= 15 per function, Cognitive Complexity <= 20, max function length <= 80 lines.

## 5. Model Precedence & Zero-Cost Delegation
- Commit messages and repetitive test digestions must be delegated to local zero-cost models (`qwen3.8:27b` / Ollama Metal) to conserve API tokens.
- Cloud models (Gemini Pro, Copilot Sonnet) are reserved for macro architecture, complex decomposition, and review gates.
