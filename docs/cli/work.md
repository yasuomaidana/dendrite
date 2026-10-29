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
| `done` | Remove a copy (refuses uncommitted/unpushed work unless --force) |
| `gc` | Remove copies that are merged (or whose upstream is gone) and clean |
| `list` | List working copies and their cleanup status |
| `new` | Create an isolated working copy (branch agent/<name>) |

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

Create an isolated working copy (branch agent/<name>)

```bash
dendrite work new <name> [flags]
```

Aliases: `create`

| Flag | Description |
| :--- | :--- |
| `--base` | Commit-ish to start from (default HEAD) |
| `--branch` | Branch to create (default agent/<name>) |
| `--no-ticket-guard` | Do not check or start the ticket in Plane (env DENDRITE_TICKET_GUARD=off) |

## Examples

```bash
dendrite work new DEN-123
dendrite work list --json
dendrite work done DEN-123 --force
dendrite work gc --dry-run
dendrite work adopt
```
