---
id: chronos
title: "dendrite chronos"
sidebar_label: "chronos"
---

# dendrite chronos

Sovereign issue tracking, Gantt alignment, and DAG scheduler

## Category

Soma & Axon Hillock

## Aliases

- `plane`
- `chronos-tracker`
- `pln`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `status` | Display synchronization status with self-hosted Plane instance |
| `replicate` | Replicate DEV_STATUS.md directly from Plane service state |
| `push-status` | Push local DEV_STATUS.md content into Plane service |
| `next` | Smart select next ticket unblocked by dependencies |
| `link` | Create dependency relationship between tickets for Gantt alignment |
| `create` | Create a new ticket placed on Plane Gantt development plan |
| `doc` | Synchronize documentation pages between Plane and local repository |
| `issues` | List open issues and milestones from Plane |

## Flags

None

## Examples

```bash
dendrite plane status
dendrite plane replicate
dendrite plane next
```
