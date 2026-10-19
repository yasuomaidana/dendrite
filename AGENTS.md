# Project Agents & Environment Orchestration

This repository is orchestrated with autonomous multi-agent loops and governed by Dendrite.

## Standard Tooling
- **Default CI/CD Assistant**: GitHub Copilot (`.github/copilot-instructions.md`)
- **Runtime Stack**: GO
- **Enforced Toolchains**: Go (`go`), Rust (`cargo`), Bun (`bun`), uv (`uv`)
- **Authoritative Issue Tracker**: Plane v2 (`http://localhost:8090`)
- **Token Governor**: LiteLLM Gateway (`http://localhost:4000`)

## Preparation & Sync
To synchronize agent rules, skills, and enforce CI/CD tooling defaults, execute:
```bash
dendrite soma prepare   # alias: dendrite env prepare
```

## Dendrite-First Workflow (all models: Claude, Antigravity/AGY, Gemini, Copilot, humans)
1. Start every session with `dendrite session resume`; it also removes merged, clean working copies.
2. Pick work from Plane with `dendrite chronos next` (alias `plane next`, or the `dendrite_ticket_smart_select` MCP tool), never from `DEV_STATUS.md`.
3. Need an isolated checkout? Run `dendrite work new <ticket>`. It creates a git worktree in `dendrite-work/` next to the main checkout. Never hand-make `git clone` or `git worktree add ../<dir>` copies: those mix with human projects and are never cleaned up.
4. Commit with `dendrite quantal` (alias `commit`; it references the ticket and pushes to origin, never to main/master).
5. Route every LLM call through Dendrite: the LiteLLM gateway (`http://localhost:4000`) for model access and `dendrite bouton jobs submit --isolate` (alias `agent jobs`) to delegate to Antigravity (AGY) or other models. Do not call provider APIs or spawn other agent CLIs directly.
6. Run `dendrite session checkpoint` after each finished ticket and before long waits.
7. When a copy is finished and merged, `dendrite work gc` (or the next resume/checkpoint) deletes it; `dendrite work done <name>` removes one explicitly.

See the `dendrite-workflow` skill for commands and cleanup rules.

## Commit & VCS Guidance
- `dendrite quantal` (alias `commit`) pushes the new commit to the configured remote (origin preferred) after every successful commit; use `--no-push` or `DENDRITE_COMMIT_PUSH=false` to opt out; it never force-pushes and refuses to push main/master.

## Session Start & Handoff
Every new session (Claude, AGY, or human) starts with:
```bash
dendrite session resume
```
Orchestrators run `dendrite session checkpoint` after each finished ticket and before long waits:
```bash
dendrite session checkpoint [--worktree dir] [--decision "..."]... [--next "DEN-XXX"]... [--note "..."]
```
