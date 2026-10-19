---
id: mesh
title: "dendrite mesh"
sidebar_label: "mesh"
---

# dendrite mesh

Distributed codebase spawner, modular REA, and repo coordinator

## Category

Axon & Myelin Sheath

## Aliases

- `dist`
- `distcodebase`
- `distributed`
- `neural-mesh`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `decompose` | Plan a forced modular decomposition into reused libraries, custom modules and satellites |
| `interop` | Coordinate a feature an owned dependency is missing |
| `interop complete` | Mark the dependency's feature as delivered |
| `interop coordinate` | Open a coordination and emit an unblocking fallback marker |
| `interop list` | List coordinations |
| `interop reconcile` | Replace the fallback in the requester with the delivered feature |
| `resolve` | Find the highest-priority library that provides a capability |
| `sources` | List codebase sources and registered libraries |
| `spawn` | Create a parallel or satellite project (git repo, workspace entry and Plane project) |

## Flags

None

## Subcommand Reference

### `dendrite mesh interop list`

List coordinations

```bash
dendrite mesh interop list
```

Aliases: `ls`

### `dendrite mesh sources`

List codebase sources and registered libraries

```bash
dendrite mesh sources
```

Aliases: `list-sources`

### `dendrite mesh spawn`

Create a parallel or satellite project (git repo, workspace entry and Plane project)

```bash
dendrite mesh spawn <project> <target-dir> [language] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--desc` | Project description |
| `--identifier` | Plane project identifier (e.g. AUTH) |
| `--no-plane` | Do not create a Plane project |
| `--satellite` | Spawn as a satellite of the current project |

## Examples

```bash
dendrite mesh sources
dendrite mesh resolve totp go
dendrite mesh decompose my-emulator ./spec.md
dendrite mesh spawn auth-svc ../auth-svc go --satellite --no-plane
```
