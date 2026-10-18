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
