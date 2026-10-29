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
| `project` | Link the repository to its Plane project and audit the project rule |
| `project audit` | Check that every editable repository is a Plane project and related repositories share a synapse |
| `project ensure` | Adopt or create the repository's Plane project, converge its states, and write .dendrite/plane.json |
| `project refresh` | Relocate or prune the entries of the global project index whose directory moved or vanished |
| `project remove` | Archive (default) or permanently delete Plane projects, keeping the local directories |
| `project show` | Print the Plane project linked to the repository |
| `push` | Push DEV_STATUS.md to Plane with Gantt timelines and dependencies |
| `relate` | Set a ticket's priority and dates and add several relations at once |
| `replicate` | Rewrite DEV_STATUS.md from the Plane board |
| `request` | Ask a synapse peer for a feature by creating a ticket in its Plane project |
| `states` | Manage the project's workflow states |
| `states reconcile` | Converge workflow states (names, colors, order) onto the canonical set |
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
| `--synapse` | Only consider requests that peers filed through this synapse (`chronos request`) |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

```bash
dendrite chronos next
dendrite chronos next --ticket DEN-123 --json
dendrite chronos next --milestone M3
```

### `dendrite chronos project audit`

Check that every editable repository is a Plane project and related repositories share a synapse

```bash
dendrite chronos project audit [flags]
```

| Flag | Description |
| :--- | :--- |
| `--owner` | Git remote owners whose repositories are editable (default: this repository's owner) |
| `--roots` | Directories to scan (default: parents of every known project and synapse member) |

```bash
dendrite chronos project audit
dendrite chronos project audit --roots ~/GoLandProjects,~/RustRoverProjects --json
```

### `dendrite chronos project ensure`

Adopt or create the repository's Plane project, converge its states, and write .dendrite/plane.json

```bash
dendrite chronos project ensure [dir] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--description` | Plane project description |
| `--identifier` | Plane project identifier (default: the linked one, else derived from the name) |
| `--import` | Markdown checklist to import as issues |
| `--name` | Plane project name (default: the repository directory name) |

```bash
dendrite chronos project ensure
dendrite chronos project ensure ../dendrite-git-rs --identifier DGIT --import DEV_STATUS.md
```

### `dendrite chronos project refresh`

Relocate or prune the entries of the global project index whose directory moved or vanished

```bash
dendrite chronos project refresh [flags]
```

| Flag | Description |
| :--- | :--- |
| `--depth` | Maximum search depth below each root |
| `--dry-run` | Show what would happen without changing anything |
| `--prune` | Remove entries whose project cannot be found |
| `--roots` | Extra directories to search (comma-separated) |

```bash
dendrite chronos project refresh --dry-run
dendrite chronos project refresh --prune --roots ~/work
```

### `dendrite chronos project remove`

Archive (default) or permanently delete Plane projects, keeping the local directories

```bash
dendrite chronos project remove <IDENT|name|dir>... [flags]
```

| Flag | Description |
| :--- | :--- |
| `--force` | Also remove projects that are members of a synapse |
| `--keep-local` | Leave the local Plane binding and index entry alone |
| `--purge` | Delete permanently instead of archiving |

```bash
dendrite chronos project remove OLDPROJ
dendrite chronos project remove ~/work/old-repo --purge --yes
```

### `dendrite chronos project show`

Print the Plane project linked to the repository

```bash
dendrite chronos project show [dir]
```

```bash
dendrite chronos project show
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

### `dendrite chronos request`

Ask a synapse peer for a feature by creating a ticket in its Plane project

```bash
dendrite chronos request <peer-alias|IDENT> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--desc` | What is needed and why (goal and acceptance criteria) |
| `--from` | Ticket in this project that depends on the request |
| `--name` | Title of the request |
| `--synapse` | The synapse the request belongs to (required when several are shared) |

```bash
dendrite chronos request oxipix --name "Expose a batch resize API" --desc "docula needs ..." --from DOC-12
```

### `dendrite chronos states reconcile`

Converge workflow states (names, colors, order) onto the canonical set

```bash
dendrite chronos states reconcile [flags]
```

Aliases: `ensure`

| Flag | Description |
| :--- | :--- |
| `--all` | Reconcile every project in the workspace |
| `--dry-run` | Show the changes without applying them |
| `--project` | Plane project UUID or identifier (default: the configured project) |

```bash
dendrite chronos states reconcile
dendrite chronos states reconcile --project DGIT --dry-run
dendrite chronos states reconcile --all
```

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
