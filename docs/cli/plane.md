---
id: plane
title: "dendrite plane"
sidebar_label: "plane"
---

# dendrite plane

Sovereign issue tracking, Gantt alignment, and DAG scheduler

## Category

Soma & Axon Hillock

## Aliases

- `chronos-tracker`
- `chronos`
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
| `states` | Reconcile workflow states (names, colors, order) onto the canonical set |

## Flags

None

## Examples

```bash
dendrite plane status
dendrite plane replicate
dendrite plane next
```
