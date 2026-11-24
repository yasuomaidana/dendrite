---
id: memory
title: "dendrite memory"
sidebar_label: "memory"
---

# dendrite memory

Agent memory: journal what is worth remembering and flush it into Engram

## Category

Cortex & Sensory Integration

## Aliases

None

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `check` | Score Claude memory and Engram, and switch the memory mode when the scores say so |
| `flush` | Send the journaled memories to Engram now |
| `mode` | Show which memory stores are in use, and why |
| `mode set` | Force a memory mode by hand (auto removes the override) |
| `save` | Journal one memory now; it reaches Engram at the next flush (session end or `memory flush`) |
| `schedule` | Install (or --remove) the weekly LaunchAgent that runs `memory check` |
| `sync` | Copy Claude Code's file memories into Engram (one way, idempotent, never changes the files) |

## Flags

None

## Subcommand Reference

### `dendrite memory check`

Score Claude memory and Engram, and switch the memory mode when the scores say so

```bash
dendrite memory check [flags]
```

| Flag | Description |
| :--- | :--- |
| `--dir` | Claude memory folder (default ~/.claude/projects/<repo>/memory) |
| `--quiet` | Print nothing unless it fails (for the weekly job) |

### `dendrite memory save`

Journal one memory now; it reaches Engram at the next flush (session end or `memory flush`)

```bash
dendrite memory save <title> <body> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--ticket` | Ticket the memory belongs to |
| `--type` | Memory type (decision, plan, note, outcome, ...) (default: note) |

```bash
dendrite memory save --type decision "Engram is the memory SoT" "measured weekly against Claude memory"
types: decision, plan, note, outcome, or any label Engram accepts
```

### `dendrite memory schedule`

Install (or --remove) the weekly LaunchAgent that runs `memory check`

```bash
dendrite memory schedule [flags]
```

| Flag | Description |
| :--- | :--- |
| `--no-load` | Write the plist without loading it |
| `--remove` | Unload and delete the weekly job |

### `dendrite memory sync`

Copy Claude Code's file memories into Engram (one way, idempotent, never changes the files)

```bash
dendrite memory sync [flags]
```

| Flag | Description |
| :--- | :--- |
| `--dir` | Claude memory folder (default ~/.claude/projects/<repo>/memory) |

## Examples

```bash
dendrite memory save --type decision "Use X" "because Y"
dendrite memory flush
dendrite memory mode
dendrite memory mode set engram
dendrite memory sync
dendrite memory check
dendrite memory schedule
```
