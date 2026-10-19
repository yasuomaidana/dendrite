---
id: saga
title: "dendrite saga"
sidebar_label: "saga"
---

# dendrite saga

Autonomous multi-turn story execution and DAG branching

## Category

Postsynaptic Effectors & Actuation

## Aliases

- `story`
- `stories`
- `synaptic-saga`
- `sty`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `abort` | Delete a story's variant worktrees and branches without merging |
| `benchmark` | Benchmark every variant worktree and recommend a winner |
| `check-contracts` | Report schema drift across Protobuf, OpenAPI, Go, Rust, TypeScript and Python (exit 1 on errors) |
| `fork` | Fork a story into one git worktree per variant |
| `resolve` | Merge the winning variant, store its benchmark, and delete the others |
| `start` | Start a story, optionally checking cross-language contracts |
| `status` | Show a story's state and variant worktrees |

## Flags

None

## Subcommand Reference

### `dendrite saga benchmark`

Benchmark every variant worktree and recommend a winner

```bash
dendrite saga benchmark <story> [flags]
```

Aliases: `bench`

| Flag | Description |
| :--- | :--- |
| `--iterations` | Iterations per variant (default: 100) |
| `--vus` | Concurrent virtual users (default: 1) |

### `dendrite saga check-contracts`

Report schema drift across Protobuf, OpenAPI, Go, Rust, TypeScript and Python (exit 1 on errors)

```bash
dendrite saga check-contracts [flags]
```

Aliases: `sync-contracts`

| Flag | Description |
| :--- | :--- |
| `--code-dir` | Code directory per language (lang=dir) (default: [go=pkg,python=python,rust=crates,typescript=src]) |
| `--schema-dir` | Contract schema directories (default: [schemas,proto,api]) |

### `dendrite saga fork`

Fork a story into one git worktree per variant

```bash
dendrite saga fork <story> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--base` | Commit to fork from (default HEAD) |
| `--variants` | Variant names (comma-separated, at least two) |

### `dendrite saga resolve`

Merge the winning variant, store its benchmark, and delete the others

```bash
dendrite saga resolve <story> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--winner` | Winning variant |

### `dendrite saga start`

Start a story, optionally checking cross-language contracts

```bash
dendrite saga start <story> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--code-dir` | Code directory per language (lang=dir) (default: [go=pkg,python=python,rust=crates,typescript=src]) |
| `--schema-dir` | Contract schema directories (default: [schemas,proto,api]) |
| `--sync-contracts` | Check contract drift and mark violations in the IDE |

## Examples

```bash
dendrite saga fork cache --variants lru,arc
dendrite saga benchmark cache --iterations 200
dendrite saga resolve cache --winner arc
dendrite saga check-contracts --json
```
