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
