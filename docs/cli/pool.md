---
id: pool
title: "dendrite pool"
sidebar_label: "pool"
---

# dendrite pool

Multi-directory dependency inference & topological DAG solver

## Category

Soma & Axon Hillock

## Aliases

- `connectome-pool`
- `plexus-pool`
- `connectome`
- `plexus`
- `plx`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `add` | Add repository directory to connectome pool |
| `rm` | Remove directory from pool |
| `list` | List registered directories and dependencies |
| `resolve` | Compute topological DAG execution order across directories |

## Flags

None

## Examples

```bash
dendrite pool list
dendrite pool resolve
```
