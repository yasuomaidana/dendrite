---
id: work
title: "dendrite work"
sidebar_label: "work"
---

# dendrite work

Manage model-owned working copies in dendrite-work/ (create, list, remove, auto-clean merged)

## Category

Soma & Axon Hillock

## Aliases

- `workcopy`
- `wc`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `adopt` | Move this repo's worktrees that live outside dendrite-work/ into it |
| `dispose` | Ask to delete what merged work left behind: repair copies and branches, finished features, a retired develop |
| `done` | Remove a copy (refuses uncommitted/unpushed work unless --force) |
| `gc` | Remove copies that are merged (or whose upstream is gone) and clean |
| `list` | List working copies and their cleanup status |
| `new` | Create an isolated working copy (branch <TICKET>-<slug>/main, or <ROOT>-<slug>/<TICKET>-<slug> for a sub-ticket) |

## Flags

None

## Subcommand Reference

### `dendrite work adopt`

Move this repo's worktrees that live outside dendrite-work/ into it

```bash
dendrite work adopt [flags]
```

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Show what would happen without changing anything |

### `dendrite work dispose`

Ask to delete what merged work left behind: repair copies and branches, finished features, a retired develop

```bash
dendrite work dispose [flags]
```

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Show what would happen without changing anything |
| `--yes` | Delete every disposable item without asking |

```bash
dendrite work dispose
dendrite work dispose --dry-run
dendrite work dispose --yes
```

### `dendrite work done`

Remove a copy (refuses uncommitted/unpushed work unless --force)

```bash
dendrite work done <name|path> [flags]
```

Aliases: `remove`, `rm`

| Flag | Description |
| :--- | :--- |
| `--force` | Remove even with uncommitted or unpushed work, and delete the branch |

### `dendrite work gc`

Remove copies that are merged (or whose upstream is gone) and clean

```bash
dendrite work gc [flags]
```

Aliases: `prune`

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Show what would happen without changing anything |

### `dendrite work list`

List working copies and their cleanup status

```bash
dendrite work list
```

Aliases: `ls`

### `dendrite work new`

Create an isolated working copy (branch <TICKET>-<slug>/main, or <ROOT>-<slug>/<TICKET>-<slug> for a sub-ticket)

```bash
dendrite work new <name> [flags]
```

Aliases: `create`

| Flag | Description |
| :--- | :--- |
| `--base` | Commit-ish to start from (default HEAD, or the parent ticket's feature branch) |
| `--branch` | Branch to create (default: named after the ticket and its parent, else <name>/main) |
| `--no-ticket-guard` | Do not check or start the ticket in Plane (env DENDRITE_TICKET_GUARD=off) |
| `--target` | Branch the work merges into (default: the trunk of the parent ticket, else the integration branch) |

## Examples

```bash
dendrite work new DEN-123
dendrite work list --json
dendrite work done DEN-123 --force
dendrite work gc --dry-run
dendrite work adopt
dendrite work dispose
```
