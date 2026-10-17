---
id: skill
title: "dendrite skill"
sidebar_label: "skill"
---

# dendrite skill

Execute skill integrations (ticket-relate, gantt-sync)

## Category

Postsynaptic Effectors & Actuation

## Aliases

None

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `ticket-relate` | Link tickets and sync dependencies to Plane |
| `gantt-sync` | Align schedule dates and milestones with Plane Gantt |

## Flags

None

## Examples

```bash
dendrite skill ticket-relate DEN-030 depends-on DEN-029
dendrite skill gantt-sync
```
