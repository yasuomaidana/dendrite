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
dendrite env prepare
```

## Commit & VCS Guidance
- `dendrite commit` pushes the new commit to the configured remote (origin preferred) after every successful commit; use `--no-push` or `DENDRITE_COMMIT_PUSH=false` to opt out; it never force-pushes and refuses to push main/master.
- `dendrite commit --synapse[=<id>]` commits the current project, then walks its synapse peers: dirty peers are committed through the same AI-assisted engine after confirmation (`--yes` skips the prompt), and clean peers ahead of their upstream are pushed.
- `dendrite sync` pushes unpushed commits and refreshes the Plane board (canonical states + DEV_STATUS when it changed) for the current project and every synapse peer, then records synapse health (`active`/`broken`). `dendrite sync --all --watch` keeps every synapse current (`DENDRITE_SYNC_INTERVAL`, default 10m).
- Plane workflow states are canonical across projects (`plane.CanonicalStates`); fix drift with `dendrite plane states reconcile --all`.

