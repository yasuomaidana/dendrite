---
id: session
title: "dendrite session"
sidebar_label: "session"
---

# dendrite session

Session checkpoint and resume for multi-agent coordination

## Category

Synaptic Cleft & Gating

## Aliases

- `handoff`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `checkpoint` | Record session state into .dendrite/session/handoff.md |
| `resume` | Resume from the handoff checkpoint and report job status deltas |

## Flags

None

## Subcommand Reference

### `dendrite session checkpoint`

Record session state into .dendrite/session/handoff.md

```bash
dendrite session checkpoint [flags]
```

| Flag | Description |
| :--- | :--- |
| `--decision` | Pending decision (repeatable) |
| `--next` | Next ticket or task (repeatable) |
| `--note` | Session note (repeatable) |
| `--path` | Destination handoff file path |
| `--worktree` | Worktree directory |

```bash
dendrite session checkpoint --worktree ../wt --decision "keep X" --next DEN-123 --note "blocked on CI"
```

### `dendrite session resume`

Resume from the handoff checkpoint and report job status deltas

```bash
dendrite session resume [flags]
```

| Flag | Description |
| :--- | :--- |
| `--path` | Handoff file path |

```bash
dendrite session resume
dendrite session resume --json --path handoff.md
```

## Examples

```bash
dendrite session resume
dendrite session checkpoint --next DEN-123 --note "waiting on review"
```
