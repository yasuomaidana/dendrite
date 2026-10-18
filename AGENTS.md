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

## Session Start & Handoff
Every new session (Claude, AGY, or human) starts with:
```bash
dendrite session resume
```
Orchestrators run `dendrite session checkpoint` after each finished ticket and before long waits:
```bash
dendrite session checkpoint [--worktree dir] [--decision "..."]... [--next "DEN-XXX"]... [--note "..."]
```

