---
id: plexus
title: "dendrite plexus"
sidebar_label: "plexus"
---

# dendrite plexus

Multi-directory dependency inference & topological DAG solver

## Category

Soma & Axon Hillock

## Aliases

- `pool`
- `connectome-pool`
- `plexus-pool`
- `connectome`
- `plx`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `add` | Register project directories in the pool |
| `list` | List pool members |
| `remove` | Remove a project from the pool |
| `resolve` | Compute the topological build order and detect dependency cycles |
| `split` | Extract a module into a new pool member (asks for the destination when --target-dir is empty) |
| `switch` | Switch how a project consumes a dependency: published, branch, local or fork |

## Flags

None

## Subcommand Reference

### `dendrite plexus list`

List pool members

```bash
dendrite plexus list
```

Aliases: `ls`

### `dendrite plexus remove`

Remove a project from the pool

```bash
dendrite plexus remove <name|path>
```

Aliases: `rm`

### `dendrite plexus split`

Extract a module into a new pool member (asks for the destination when --target-dir is empty)

```bash
dendrite plexus split [flags]
```

| Flag | Description |
| :--- | :--- |
| `--from` | Project to split (default: .) |
| `--module` | Module path inside the project |
| `--name` | Name of the new pool member |
| `--no-register-pool` | Do not add the new member to the pool |
| `--no-wire-local` | Do not switch the parent to the local dependency |
| `--target-dir` | Destination directory |

### `dendrite plexus switch`

Switch how a project consumes a dependency: published, branch, local or fork

```bash
dendrite plexus switch <dependency> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--branch` | Git branch (mode branch) |
| `--commit` | Git commit (mode branch) |
| `--fork-branch` | Fork branch (mode fork) |
| `--fork-url` | Fork repository URL (mode fork) |
| `--in` | Project whose manifest is rewritten (default: .) |
| `--local-path` | Local checkout (mode local) |
| `--mode` | Dependency mode: published, branch, local or fork |
| `--no-sync-ide` | Do not update the JetBrains scopes |
| `--version` | Published version (mode published) |

## Examples

```bash
dendrite plexus add ../core ../api
dendrite plexus resolve --json
dendrite plexus switch core-lib --in ../api --mode local --local-path ../core
dendrite plexus split --module pkg/auth --name auth-svc
```
