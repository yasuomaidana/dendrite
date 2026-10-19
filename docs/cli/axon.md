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
| `advisor` | Probe compute nodes and recommend a service placement |
| `inspect` | Show stack placements and compose profiles for a scenario (default local-hybrid) |

## Flags

None

## Subcommand Reference

### `dendrite axon advisor`

Probe compute nodes and recommend a service placement

```bash
dendrite axon advisor [flags]
```

Aliases: `advise`

| Flag | Description |
| :--- | :--- |
| `--nodes` | Comma-separated nodes as id=host (or just host) (default: local=localhost) |

### `dendrite axon inspect`

Show stack placements and compose profiles for a scenario (default local-hybrid)

```bash
dendrite axon inspect [scenario]
```

Aliases: `show`

## Examples

```bash
dendrite axon advisor --nodes local=localhost,gpu=10.0.0.5
dendrite axon advisor --json
dendrite axon inspect distributed
```
