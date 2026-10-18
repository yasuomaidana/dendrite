---
id: axon
title: "dendrite axon"
sidebar_label: "axon"
---

# dendrite axon

Multi-node placement models and hardware service router

## Category

Axon & Myelin Sheath

## Aliases

- `topology`
- `axon-pathway`
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
