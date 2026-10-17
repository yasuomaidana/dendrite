---
id: doc
title: "dendrite doc"
sidebar_label: "doc"
---

# dendrite doc

Docs-as-Code reflection, Docusaurus synthesis and portal

## Category

Cortex & Sensory Integration

## Aliases

- `docs`
- `engram-codex`
- `codex`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `generate` | Synthesize Docusaurus site or reference documentation |
| `audit` | Audit documentation coverage and completeness against registries |
| `docusaurus` | Complete Docusaurus site operations (generate, init, share, audit, serve, build) |
| `reflect` | Reflect documentation taxonomy from reference to target directory |
| `plane` | Synchronize documentation pages between Plane and local repository |

## Flags

| Flag | Description |
| :--- | :--- |
| `--reference` | Generate per-command and per-tool reference pages from live registries |
| `--strict` | Exit 1 if any registered command or tool is undocumented or stale |
| `--docs` | Documentation source directory (default: docs) |
| `--out` | Target output directory |

## Examples

```bash
dendrite docs generate --reference
dendrite docs audit --strict
dendrite doc docusaurus serve --port 3000
```
