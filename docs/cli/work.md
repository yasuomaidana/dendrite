---
id: work
title: "dendrite work"
sidebar_label: "work"
---

# dendrite work

Manage model-owned working copies in dendrite-work/ (create, list, remove, auto-clean merged)

## Category

Synaptic Cleft & Gating

## Aliases

- `workcopy`
- `wc`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `new` | Create an isolated working copy in dendrite-work/ on branch agent/<name> |
| `list` | List working copies with dirty, unpushed, merged and upstream-gone status |
| `done` | Remove one copy (refuses uncommitted or unpushed work without --force) |
| `gc` | Remove copies that are merged (or whose upstream is gone) and clean |
| `adopt` | Move this repository's worktrees that live outside dendrite-work/ into it |

## Flags

| Flag | Description |
| :--- | :--- |
| `--branch` | Branch for a new copy (default agent/<name>) |
| `--base` | Commit-ish a new copy starts from (default HEAD) |
| `--force` | Remove a copy even with unsaved work, deleting its branch |
| `--dry-run` | Report what gc or adopt would do without changing anything |
| `--json` | Output the copy list as JSON |

## Examples

```bash
dendrite work new DEN-123
dendrite work list
dendrite work gc --dry-run
dendrite work adopt --dry-run
```
