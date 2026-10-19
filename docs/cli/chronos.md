---
id: chronos
title: "dendrite chronos"
sidebar_label: "chronos"
---

# dendrite chronos

Sovereign issue tracking, Gantt alignment, and DAG scheduler

## Category

Terminal Boutons & Vesicles

## Aliases

- `plane`
- `chronos-tracker`
- `pln`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `create` | Create a ticket, place it on the Gantt plan and link its blockers |
| `gantt-sync` | Align milestone dates, ticket schedules and dependency arrows on the Plane Gantt view |
| `issues` | List the project's Plane issues |
| `link` | Add one relation between two tickets (blocking, blocked_by, relates_to, duplicate, start_before, start_after, finish_before, finish_after) |
| `modules` | List Plane modules (milestones) |
| `next` | Pick the next ticket by priority and DAG reachability |
| `pages` | List Plane documentation pages |
| `push` | Push DEV_STATUS.md to Plane with Gantt timelines and dependencies |
| `relate` | Set a ticket's priority and dates and add several relations at once |
| `replicate` | Rewrite DEV_STATUS.md from the Plane board |
| `states` | Manage the project's workflow states |
| `states ensure` | Create any missing canonical lifecycle states |
| `ticket` | Read or set ticket status in DEV_STATUS.md and Plane |
| `ticket list` | List tickets from DEV_STATUS.md (falls back to Plane) |
| `ticket set` | Set a ticket's status in Plane and DEV_STATUS.md (backlog, todo, in progress, to verify, done) |
| `update` | Rename a ticket or replace its description |

## Flags

None

## Subcommand Reference

### `dendrite chronos create`

Create a ticket, place it on the Gantt plan and link its blockers

```bash
dendrite chronos create [flags]
```

| Flag | Description |
| :--- | :--- |
| `--blocked-by, -b` | Blocking tickets or issue UUIDs (comma-separated) |
| `--desc, -d` | Markdown description (context and acceptance criteria) |
| `--desc-file, -f` | Read the Markdown description from a file |
| `--estimate, -e` | Estimate in points; sets the Gantt duration |
| `--milestone, -m` | Milestone (Plane module) name or ID; created when missing |
| `--name, -n` | Ticket title, e.g. "[DEN-123] Title" |
| `--priority, -p` | Priority (urgent, high, medium, low, none) (default: medium) |

```bash
dendrite chronos create --name "[DEN-123] Title" --desc-file spec.md
dendrite chronos create --name "[DEN-124] Title" --desc "Goal and acceptance criteria" --milestone M3 --priority high --estimate 5 --blocked-by DEN-100,DEN-101
```

### `dendrite chronos issues`

List the project's Plane issues

```bash
dendrite chronos issues
```

Aliases: `list`, `ls`

### `dendrite chronos link`

Add one relation between two tickets (blocking, blocked_by, relates_to, duplicate, start_before, start_after, finish_before, finish_after)

```bash
dendrite chronos link <ticket-a> <relation> <ticket-b>
```

```bash
dendrite chronos link DEN-123 blocked_by DEN-100
dendrite chronos link DEN-123 relates_to DEN-950
```

### `dendrite chronos modules`

List Plane modules (milestones)

```bash
dendrite chronos modules
```

Aliases: `milestones`

### `dendrite chronos next`

Pick the next ticket by priority and DAG reachability

```bash
dendrite chronos next [flags]
```

| Flag | Description |
| :--- | :--- |
| `--milestone, -m` | Only consider tickets whose name contains this milestone |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

```bash
dendrite chronos next
dendrite chronos next --ticket DEN-123 --json
dendrite chronos next --milestone M3
```

### `dendrite chronos push`

Push DEV_STATUS.md to Plane with Gantt timelines and dependencies

```bash
dendrite chronos push [flags]
```

Aliases: `push-status`

| Flag | Description |
| :--- | :--- |
| `--file, -f` | Status file to read (default: DEV_STATUS.md) |

### `dendrite chronos relate`

Set a ticket's priority and dates and add several relations at once

```bash
dendrite chronos relate [flags]
```

| Flag | Description |
| :--- | :--- |
| `--blocked-by, -b` | Tickets that block this one (comma-separated) |
| `--blocking` | Tickets this one blocks (comma-separated) |
| `--priority, -p` | New priority (urgent, high, medium, low, none) |
| `--reason` | Why the relations exist (echoed for the record) |
| `--relates-to, -r` | Related tickets (comma-separated) |
| `--start, -s` | Start date (YYYY-MM-DD) |
| `--target` | Target date (YYYY-MM-DD) |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

```bash
dendrite chronos relate --ticket DEN-123 --priority high --blocked-by DEN-100,DEN-101 --relates-to DEN-950
```

### `dendrite chronos replicate`

Rewrite DEV_STATUS.md from the Plane board

```bash
dendrite chronos replicate [flags]
```

Aliases: `pull`, `status`, `pull-status`

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Show what would happen without changing anything |
| `--file, -f` | Status file to write (default: DEV_STATUS.md) |

### `dendrite chronos states ensure`

Create any missing canonical lifecycle states

```bash
dendrite chronos states ensure [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Plane project UUID (default: the configured project) |

### `dendrite chronos ticket list`

List tickets from DEV_STATUS.md (falls back to Plane)

```bash
dendrite chronos ticket list
```

Aliases: `ls`

### `dendrite chronos ticket set`

Set a ticket's status in Plane and DEV_STATUS.md (backlog, todo, in progress, to verify, done)

```bash
dendrite chronos ticket set <ticket> <status>
```

```bash
dendrite chronos ticket set DEN-123 done
dendrite chronos ticket set DEN-123 "to verify"
```

### `dendrite chronos update`

Rename a ticket or replace its description

```bash
dendrite chronos update [flags]
```

| Flag | Description |
| :--- | :--- |
| `--desc, -d` | Markdown description (context and acceptance criteria) |
| `--desc-file, -f` | Read the Markdown description from a file |
| `--name, -n` | New ticket title |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

```bash
dendrite chronos update --ticket DEN-123 --desc-file spec.md
dendrite chronos update -t DEN-123 --name "[DEN-123] New title"
```

## Examples

```bash
dendrite chronos next --json
dendrite chronos create --name "[DEN-123] Title" --desc-file spec.md --blocked-by DEN-100
dendrite chronos link DEN-123 relates_to DEN-100
dendrite chronos ticket set DEN-123 done
dendrite chronos replicate --dry-run
```
