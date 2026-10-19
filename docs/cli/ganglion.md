---
id: ganglion
title: "dendrite ganglion"
sidebar_label: "ganglion"
---

# dendrite ganglion

Manage multi-project isolated workspaces

## Category

Soma & Axon Hillock

## Aliases

- `project`
- `ganglion-workspace`
- `prj`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `add` | Register a project and its isolated data directory |
| `list` | List registered projects (* marks the active one) |
| `remove` | Unregister a project |
| `switch` | Make a registered project the active context |

## Flags

None

## Subcommand Reference

### `dendrite ganglion list`

List registered projects (* marks the active one)

```bash
dendrite ganglion list
```

Aliases: `ls`

### `dendrite ganglion remove`

Unregister a project

```bash
dendrite ganglion remove <name> [flags]
```

Aliases: `rm`

| Flag | Description |
| :--- | :--- |
| `--purge` | Also delete the project's data directory |

## Examples

```bash
dendrite ganglion add dendrite ~/GoLandProjects/Dendrite
dendrite ganglion switch dendrite
dendrite ganglion list
dendrite ganglion remove old-project --purge
```
