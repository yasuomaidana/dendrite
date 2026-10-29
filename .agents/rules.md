# Autonomous Agent Operating Rules

## Authoritative Project Management & Plane Ownership
- Plane v2 (`http://localhost:8090`) is the authoritative owner of ticket state.
- NEVER use `DEV_STATUS.md` to decide what to work on; query Plane via `dendrite chronos next` or `dendrite_ticket_smart_select`.
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
3. Every change follows one ticket through its lifecycle. Pull the ticket (`dendrite chronos next`) or create it (`dendrite chronos create --name "..." --desc "goal and acceptance criteria"`), design the plan, then start it with `dendrite work new <ticket>`, which moves it to In Progress; `dendrite quantal` refuses a commit whose ticket is not In Progress. When the plan ends, set the next state with `dendrite chronos ticket set <ticket> <state>` (or the `dendrite_plane_ticket_set` MCP tool): `to verify` when the work is finished (agents never set `done`: a human signs off), `blocked` when it waits on something (write what in the ticket), `cancelled` when it is dropped, `backlog` when it is parked. A challenge found mid-plan is a new ticket linked with `dendrite chronos link`. Bypass the guard only deliberately, with `--no-ticket-guard`.
4. Every repository we can modify is a Plane project: run `dendrite chronos project ensure` in it (it writes the local `.dendrite/plane.json` that `dendrite chronos`, `sync` and agents read; never commit anything under `.dendrite/`, and in a fresh clone this command recovers the project's identity). Repositories that depend on each other belong to the same synapse (`dendrite synapse create`). `dendrite chronos project audit` lists any repository that breaks this rule.
5. Work on your own branch, never directly on `develop`, `main` or `master`: run `dendrite work new <ticket>` for each ticket. It creates a git worktree in `dendrite-work/` next to the main checkout, on branch `agent/<ticket>`. Never hand-make `git clone` or `git worktree add ../<dir>` copies: those mix with human projects and are never cleaned up.
6. Commit with `dendrite quantal` (alias `commit`; it references the ticket and pushes your branch to origin, never to main/master).
7. When the ticket is finished, merge your branch into `develop` and push, so everyone's work syncs through `develop`: from your copy run `git fetch origin && git merge origin/develop`, fix conflicts and re-run the tests on your branch, then `git push origin HEAD:develop`. If the push is rejected, someone else merged first: fetch, merge and push again. Never merge into or push `main`/`master`.
8. Explore with Dendrite before any raw text search, cheapest first: `dendrite rag search`/`dendrite_rag_retrieve` (detail `map`, then `dendrite rag show`/`dendrite_rag_show` for the bodies you need), `dendrite rag symbols` for definitions and references, `dendrite graph` (`dendrite_graph_explain`, `dendrite_graph_path`) for callers and structure, `dendrite rag history` for past changes, and `dendrite ask` for questions. `dendrite rag grep` is the exact-pattern fallback (index-aware, capped). Use `rg`/`grep`/`find` only when those return nothing, and then scoped to a path with a result cap, because unscoped `rg` output costs far more tokens than a retrieval map.
9. Questions about tickets (status, blockers, what a ticket is for) go to Plane through Dendrite: `dendrite ask --corpus tickets "..."` or `dendrite rag search --corpus tickets "..."` (MCP: `dendrite_rag_retrieve` with corpus `tickets`) for open questions, and `dendrite chronos`, `dendrite_plane_issues` or `dendrite_ticket_smart_select` for exact lookups. Never use `DEV_STATUS.md` or a search of the repository for this.
10. Route every LLM call through Dendrite: the LiteLLM gateway (`http://localhost:4000`) for model access and `dendrite bouton jobs submit --isolate` (alias `agent jobs`) to delegate to Antigravity (AGY) or other models. Do not call provider APIs or spawn other agent CLIs directly.
11. Run `dendrite session checkpoint` after each finished ticket and before long waits.
12. When a copy is finished and merged, `dendrite work gc` (or the next resume/checkpoint) deletes it; `dendrite work done <name>` removes one explicitly.
