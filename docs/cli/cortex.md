---
id: cortex
title: "dendrite cortex"
sidebar_label: "cortex"
---

# dendrite cortex

Embedded Web Control Cockpit and telemetry server

## Category

Cortex & Sensory Integration

## Aliases

- `ui`
- `cortex-cockpit`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `sync` | Synchronize UI component design tokens and Penpot frames |

## Flags

| Flag | Description |
| :--- | :--- |
| `--port` | Local HTTP port to bind Web UI (default: 3456) |
| `--daemon` | Run in continuous background daemon mode |
| `--file` | Path to UI component file for synchronization |
| `--ticket` | Associated ticket ID for UI synchronization |

## Examples

```bash
dendrite ui
dendrite ui --port 8080 --daemon
dendrite ui sync --file src/Button.tsx --ticket DEN-810
```
