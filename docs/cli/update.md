---
id: update
title: "dendrite update"
sidebar_label: "update"
---

# dendrite update

Update dendrite from a release, or rebuild it and its synapse siblings from local sources (--dev)

## Category

Cortex & Sensory Integration

## Aliases

- `upgrade`
- `self-update`

## Subcommands

None

## Flags

| Flag | Description |
| :--- | :--- |
| `--autostash` | Stash uncommitted changes around member fast-forwards |
| `--dev` | Build from local sources instead of downloading a release |
| `--no-members` | Do not build sibling member binaries |
| `--no-sync` | Do not fetch or fast-forward synapse members |
| `--source-dir` | Dendrite checkout to build (default: $DENDRITE_SRC or the enclosing git repo) |
| `--synapse` | Sync and build only this synapse's members (repeatable) |
| `--target` | Binary to replace (default: the running dendrite, or $GOBIN/dendrite under go run) |

## Examples

```bash
dendrite update
dendrite update --dev
dendrite update --dev --synapse dendrite-ecosystem --no-members
```
