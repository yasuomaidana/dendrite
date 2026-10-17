---
id: topology
title: "dendrite topology"
sidebar_label: "topology"
---

# dendrite topology

Multi-node placement models and hardware service router

## Category

Axon & Myelin Sheath

## Aliases

- `axon-pathway`
- `axon`
- `top`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `advisor` | Run dynamic service placement advisor across compute nodes |
| `inspect` | Inspect and validate topology scenario placements |

## Flags

| Flag | Description |
| :--- | :--- |
| `--nodes` | Comma-separated node definitions (e.g. 'local=localhost,worker=10.0.0.2') |
| `--json` | Output placement plan in JSON format |

## Examples

```bash
dendrite topology advisor
dendrite topology inspect local-hybrid
```
