# Autonomous Agent Operating Rules

## Authoritative Project Management & Plane Ownership
- Plane v2 (`http://localhost:8090`) is the authoritative owner of ticket state.
- NEVER use `DEV_STATUS.md` to decide what to work on; query Plane via `./bin/dendrite plane next` or `dendrite_ticket_smart_select`.
- Completed features must transition to `To Verify` (intermediate verification state) for human sign-off, never directly to `Done`.

## Architecture & CI/CD Tooling Defaults
- Default CI/CD & Assistant Tool: **GitHub Copilot** (`.github/copilot-instructions.md`).
- Primary Stack: `go`
- Standardized Toolsets:
  - **Go**: `go test -v -race`, `golangci-lint`
  - **Rust**: `cargo test`, `cargo build`, `cargo clippy`
  - **JavaScript / TypeScript**: `bun test`, `bun run build` (zero npm)
  - **Python**: `uv run`, `uv pip` (zero legacy pip)
- Statement coverage threshold: **>= 85.0%**.
- Anti-mock policy: genuine structs and Testcontainers only.
