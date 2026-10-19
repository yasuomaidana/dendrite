---
id: github-sync
title: "dendrite github-sync"
sidebar_label: "github-sync"
---

# dendrite github-sync

Mirror Plane verification state to GitHub: open HITL issues for To Verify tickets, close them when Done

## Category

Axon & Myelin Sheath

## Aliases

- `sync-github`
- `saltatory-sync`
- `salt-sync`
- `gsync`

## Subcommands

None

## Flags

| Flag | Description |
| :--- | :--- |
| `--repo` | Target GitHub repository (owner/repo); default: the origin remote |
| `--dry-run` | Show the planned issue changes without modifying GitHub |
| `--ticket` | Limit the sync to these ticket IDs (comma-separated) |
| `--json` | Output the sync result as JSON |

## Examples

```bash
dendrite github-sync --ticket DEN-123
dendrite github-sync --dry-run
dendrite github-sync
```
