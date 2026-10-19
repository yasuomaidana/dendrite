---
id: codex
title: "dendrite codex"
sidebar_label: "codex"
---

# dendrite codex

Docs-as-Code reflection, Docusaurus synthesis and portal

## Category

Cortex & Sensory Integration

## Aliases

- `doc`
- `docs`
- `engram-codex`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `audit` | Audit the reference docs against the code (--strict fails on drift), or the site's coverage |
| `generate` | Generate the Docusaurus site, or with --reference the CLI and MCP reference pages |
| `plane` | Sync the docs directory with Plane pages |
| `plane pull` | Write Plane pages into the docs directory |
| `plane push` | Publish local docs as Plane pages |
| `plane sync` | Push and pull, newest wins |
| `reflect` | Mirror a reference docs taxonomy onto another source tree |
| `site` | Scaffold, share, preview and build the Docusaurus site |
| `site build` | Run the production npm build of the site |
| `site init` | Scaffold a blank Docusaurus project |
| `site serve` | Serve the site preview and sharing portal in the foreground (Ctrl+C stops it) |
| `site share` | Export a portable documentation bundle with a SHA-256 manifest |

## Flags

None

## Subcommand Reference

### `dendrite codex audit`

Audit the reference docs against the code (--strict fails on drift), or the site's coverage

```bash
dendrite codex audit [flags]
```

| Flag | Description |
| :--- | :--- |
| `--docs` | Documentation directory (default: docs) |
| `--reference` | Audit the reference docs without failing |
| `--root` | Project root for the site audit (default: .) |
| `--strict` | Audit the reference docs and exit 1 when any page is missing or stale |

### `dendrite codex generate`

Generate the Docusaurus site, or with --reference the CLI and MCP reference pages

```bash
dendrite codex generate [flags]
```

| Flag | Description |
| :--- | :--- |
| `--docs` | Documentation directory (default: docs) |
| `--out` | Site output directory (default: website) |
| `--port` | Preview port written to the site config (default: 3000) |
| `--reference` | Write docs/cli and docs/mcp from the live command tree and MCP tools |
| `--tier` | Sharing tier: public, internal, federated or private (default: federated) |
| `--title` | Site title (default: Dendrite Documentation) |

### `dendrite codex reflect`

Mirror a reference docs taxonomy onto another source tree

```bash
dendrite codex reflect [flags]
```

| Flag | Description |
| :--- | :--- |
| `--reference` | Reference docs directory (default: docs) |
| `--source` | Source tree to document (default: .) |
| `--target` | Output directory (default: reflected_docs) |

### `dendrite codex site`

Scaffold, share, preview and build the Docusaurus site

```bash
dendrite codex site
```

Aliases: `docusaurus`

### `dendrite codex site build`

Run the production npm build of the site

```bash
dendrite codex site build [flags]
```

| Flag | Description |
| :--- | :--- |
| `--site` | Docusaurus site directory (default: website) |

### `dendrite codex site init`

Scaffold a blank Docusaurus project

```bash
dendrite codex site init [flags]
```

| Flag | Description |
| :--- | :--- |
| `--out` | Directory to create (default: website) |

### `dendrite codex site serve`

Serve the site preview and sharing portal in the foreground (Ctrl+C stops it)

```bash
dendrite codex site serve [flags]
```

| Flag | Description |
| :--- | :--- |
| `--port` | Listen port (default: 3000) |
| `--site` | Docusaurus site directory (default: website) |

### `dendrite codex site share`

Export a portable documentation bundle with a SHA-256 manifest

```bash
dendrite codex site share [flags]
```

| Flag | Description |
| :--- | :--- |
| `--out` | Bundle path (default inside the site) |
| `--site` | Docusaurus site directory (default: website) |

## Examples

```bash
dendrite codex generate --reference
dendrite codex audit --strict
dendrite codex generate --out website --tier internal
dendrite codex site serve --port 3000
dendrite codex plane push --dir docs
```
