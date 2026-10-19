---
id: arbor
title: "dendrite arbor"
sidebar_label: "arbor"
---

# dendrite arbor

Intent-driven CFG/AST reachability slicing, debloater, transpile & fuse

## Category

Dendritic Arbor

## Aliases

- `query`
- `dendritic-arbor`
- `dend`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `fuse` | Fuse several artifacts into one pool member with shared libraries deduplicated |
| `reuse` | Pick a library for a capability with the 3-tier reuse audit (owned, reference, ecosystem) |
| `slice` | Keep only the code reachable from an entrypoint for an intent |
| `swap` | Swap a graphics subsystem behind a generated trait boundary |
| `transpile` | Transpile an artifact into safe idiomatic Rust with differential tests |

## Flags

None

## Subcommand Reference

### `dendrite arbor fuse`

Fuse several artifacts into one pool member with shared libraries deduplicated

```bash
dendrite arbor fuse [flags]
```

| Flag | Description |
| :--- | :--- |
| `--lang` | Target language (default: rust) |
| `--name` | Name of the fused pool member |
| `--no-register-pool` | Do not add the fused member to the pool |
| `--sources` | Artifacts to fuse (comma-separated, at least two) |
| `--target-dir` | Destination directory (prompted when empty) |

### `dendrite arbor reuse`

Pick a library for a capability with the 3-tier reuse audit (owned, reference, ecosystem)

```bash
dendrite arbor reuse [flags]
```

| Flag | Description |
| :--- | :--- |
| `--capability` | Capability to find a library for |
| `--intent` | What the capability is used for |
| `--lang` | Target language (default: rust) |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) (default: DEN-058) |

### `dendrite arbor slice`

Keep only the code reachable from an entrypoint for an intent

```bash
dendrite arbor slice [intent...] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--audit-library-reuse` | Also run the 3-tier library reuse audit on the result |
| `--direction` | Reachability direction: forward, backward or bidirectional (default: forward) |
| `--entrypoint` | Symbol to slice from |
| `--output-dir` | Write the generated project or files here |
| `--sources` | Source tree to slice (default: .) |

### `dendrite arbor swap`

Swap a graphics subsystem behind a generated trait boundary

```bash
dendrite arbor swap [flags]
```

| Flag | Description |
| :--- | :--- |
| `--from` | Subsystem to replace: opengl, directx or vulkan (default: opengl) |
| `--lang` | Target language (default: rust) |
| `--output-dir` | Write the generated project or files here |
| `--sources` | Source tree containing the subsystem (default: .) |
| `--to` | Replacement subsystem: wgpu or threejs (default: wgpu) |

### `dendrite arbor transpile`

Transpile an artifact into safe idiomatic Rust with differential tests

```bash
dendrite arbor transpile [prompt...] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--no-std` | Emit a no_std crate |
| `--output-dir` | Write the generated project or files here |
| `--sources` | Artifact to transpile (default: .) |
| `--to` | Target language (default: rust) |

## Examples

```bash
dendrite arbor slice --entrypoint ParseConfig "keep only YAML parsing"
dendrite arbor swap --sources ./engine --from opengl --to wgpu
dendrite arbor transpile --sources ./lib.c --no-std "fixed-point math"
dendrite arbor fuse --sources svc-a,svc-b --name unified
dendrite arbor reuse --capability totp --lang go
```
