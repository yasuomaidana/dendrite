# Autonomous Agent Operating Rules

## Authoritative Project Management & Plane Ownership
- Plane v2 (`http://localhost:8090`) is the authoritative owner of ticket state.
- NEVER use `DEV_STATUS.md` to decide what to work on; query Plane via `./bin/dendrite chronos next` or `dendrite_ticket_smart_select`.
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

## Dendrite-First Workflow (every model and session)
1. Start every session with `dendrite session resume`; it also removes merged, clean working copies.
2. Pick work from Plane with `dendrite chronos next` (alias `plane next`, or the `dendrite_ticket_smart_select` MCP tool), never from `DEV_STATUS.md`.
3. Every repository we can modify is a Plane project: run `dendrite chronos project ensure` in it (it writes the committed `.dendrite/plane.json` that `dendrite chronos`, `sync` and agents read). Repositories that depend on each other belong to the same synapse (`dendrite synapse create`). `dendrite chronos project audit` lists any repository that breaks this rule.
4. Need an isolated checkout? Run `dendrite work new <ticket>`. It creates a git worktree in `dendrite-work/` next to the main checkout. Never hand-make `git clone` or `git worktree add ../<dir>` copies: those mix with human projects and are never cleaned up.
5. Commit with `dendrite quantal` (alias `commit`; it references the ticket and pushes to origin, never to main/master).
6. Route every LLM call through Dendrite: the LiteLLM gateway (`http://localhost:4000`) for model access and `dendrite bouton jobs submit --isolate` (alias `agent jobs`) to delegate to Antigravity (AGY) or other models. Do not call provider APIs or spawn other agent CLIs directly.
7. Run `dendrite session checkpoint` after each finished ticket and before long waits.
8. When a copy is finished and merged, `dendrite work gc` (or the next resume/checkpoint) deletes it; `dendrite work done <name>` removes one explicitly.
