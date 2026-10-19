---
id: cognition
title: "dendrite cognition"
sidebar_label: "cognition"
---

# dendrite cognition

Cognitive complexity analyzer, structural anti-pattern scanner, and policy router

## Category

Glial Regulation & Pruning

## Aliases

- `analyze`
- `cognitive-analysis`
- `anlz`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `mode` | Show or set the analysis operating mode |
| `pause` | Pause analysis and persist the active chunk's progress |
| `policy` | Show the scheduler policy, or change it with flags |
| `resume` | Resume analysis in the active mode |
| `run` | Run one analysis pass on a chunk under scheduler governance |
| `status` | Show the scheduler's tier, windows, countdowns and the governor state |
| `switch` | Set runtime feature switches |

## Flags

None

## Subcommand Reference

### `dendrite cognition pause`

Pause analysis and persist the active chunk's progress

```bash
dendrite cognition pause [flags]
```

| Flag | Description |
| :--- | :--- |
| `--chunk, -c` | Active chunk ID |
| `--project` | Project of the active chunk |

### `dendrite cognition policy`

Show the scheduler policy, or change it with flags

```bash
dendrite cognition policy [flags]
```

| Flag | Description |
| :--- | :--- |
| `--allow-zero` | Allow draining the quota below the reserve floor |
| `--mode` | Mode: auto, always, night, sprint or paused |
| `--night-window` | Zero-cost local window, HH:MM-HH:MM |
| `--reserve-floor` | Safety reserve as a fraction (0.0-1.0) |
| `--sprint-before-reset` | Cloud sprint window before the reset |
| `--weekly-reset` | Weekly quota reset, e.g. "MON 09:00" |

### `dendrite cognition resume`

Resume analysis in the active mode

```bash
dendrite cognition resume [flags]
```

| Flag | Description |
| :--- | :--- |
| `--chunk, -c` | Active chunk ID |
| `--project` | Project of the active chunk |

### `dendrite cognition run`

Run one analysis pass on a chunk under scheduler governance

```bash
dendrite cognition run [flags]
```

| Flag | Description |
| :--- | :--- |
| `--chunk, -c` | Chunk ID (default: project-ast-chunk) |
| `--project` | Project (default: dendrite) |
| `--simulate-night` | Run as if inside the night window |
| `--simulate-sprint` | Run as if inside the sprint window |

### `dendrite cognition switch`

Set runtime feature switches

```bash
dendrite cognition switch [flags]
```

| Flag | Description |
| :--- | :--- |
| `--allow-zero` | Allow-zero drain override: on or off |

## Examples

```bash
dendrite cognition status --json
dendrite cognition policy --night-window 22:00-06:00 --reserve-floor 0.25
dendrite cognition switch --allow-zero off
dendrite cognition mode paused
dendrite cognition run --chunk ast-1 --simulate-night
```
