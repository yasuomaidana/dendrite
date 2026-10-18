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
| `add` | Register a new workspace and prepare isolated state |
| `switch` | Switch active workspace context (< 50ms) |
| `list` | List all registered workspaces |
| `remove` | Unregister workspace and optionally purge data |

## Flags

| Flag | Description |
| :--- | :--- |
| `--purge` | Purge persistent data directory when removing workspace |

## Examples

```bash
dendrite project add backend /path/to/repo
dendrite project switch backend
dendrite project list
dendrite project remove backend --purge
```
