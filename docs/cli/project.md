---
id: project
title: "dendrite project"
sidebar_label: "project"
---

# dendrite project

Manage multi-project isolated workspaces

## Category

Soma & Axon Hillock

## Aliases

- `ganglion-workspace`
- `ganglion`
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
