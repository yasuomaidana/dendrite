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
| `canvas` | Interactive writing canvas and Penpot UI creative action bridge |
| `canvas actions` | List the supported canvas actions |
| `canvas dispatch` | Run one canvas writing or UI creative action |
| `canvas list` | List ticket-to-feature relations |
| `canvas relate` | Bind a Plane ticket to a UI feature and Penpot frame |
| `canvas sync` | Sync a UI component with its Penpot frame and Plane ticket |
| `canvas update-state` | Move a ticket to a new state and record a note |
| `serve` | Run the web cockpit and telemetry server in the foreground (Ctrl+C stops it) |
| `sync` | Extract a UI component's tokens and props and sync them to Penpot and the Plane ticket |

## Flags

None

## Subcommand Reference

### `dendrite cortex canvas`

Interactive writing canvas and Penpot UI creative action bridge

```bash
dendrite cortex canvas
```

Aliases: `creative-canvas`

```bash
dendrite cortex canvas actions
dendrite cortex canvas dispatch --action WRITE_PLAN --prompt "Q4 roadmap"
dendrite cortex canvas relate --ticket DEN-807 --feature login --frame login-v2
dendrite cortex canvas update-state --ticket DEN-807 --state "To Verify"
```

### `dendrite cortex canvas actions`

List the supported canvas actions

```bash
dendrite cortex canvas actions
```

Aliases: `list-actions`

### `dendrite cortex canvas dispatch`

Run one canvas writing or UI creative action

```bash
dendrite cortex canvas dispatch [flags]
```

Aliases: `run`

| Flag | Description |
| :--- | :--- |
| `--action` | Canvas action ID (see 'canvas actions') (default: WRITE_PLAN) |
| `--component` | Component name for UI generation (default: CanvasComponent) |
| `--content` | Existing content to refine or critique |
| `--file, -f` | Target UI component file |
| `--framework` | Frontend framework: svelte, react or html (default: svelte) |
| `--mode` | Writing mode: DRAFT, REFINE, EXPAND, CONDENSE, CHANGE_TONE, CRITIQUE or FORMAT (default: DRAFT) |
| `--prompt` | Generation prompt or acceptance criteria |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |
| `--title` | Artifact title |

### `dendrite cortex canvas list`

List ticket-to-feature relations

```bash
dendrite cortex canvas list
```

Aliases: `ls`

### `dendrite cortex canvas relate`

Bind a Plane ticket to a UI feature and Penpot frame

```bash
dendrite cortex canvas relate [flags]
```

| Flag | Description |
| :--- | :--- |
| `--component` | Component file path |
| `--desc` | Description or acceptance context |
| `--feature` | Feature name |
| `--frame` | Penpot frame ID or slug |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

### `dendrite cortex canvas sync`

Sync a UI component with its Penpot frame and Plane ticket

```bash
dendrite cortex canvas sync [flags]
```

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Show what would happen without changing anything |
| `--file, -f` | UI component file |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

### `dendrite cortex canvas update-state`

Move a ticket to a new state and record a note

```bash
dendrite cortex canvas update-state [flags]
```

| Flag | Description |
| :--- | :--- |
| `--comment` | Status note or verification remark |
| `--no-plane` | Do not push the change to Plane |
| `--penpot-url` | Penpot frame deep link |
| `--state` | Target state (In Progress, To Verify, Done) |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

### `dendrite cortex serve`

Run the web cockpit and telemetry server in the foreground (Ctrl+C stops it)

```bash
dendrite cortex serve [flags]
```

Aliases: `start`

| Flag | Description |
| :--- | :--- |
| `--port` | Listen port (default: 3456) |

### `dendrite cortex sync`

Extract a UI component's tokens and props and sync them to Penpot and the Plane ticket

```bash
dendrite cortex sync [flags]
```

Aliases: `penpot`

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Show what would happen without changing anything |
| `--endpoint, -e` | Penpot URL (default: http://localhost:9000) |
| `--file, -f` | UI component file |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) (default: DEN-810) |

## Examples

```bash
dendrite cortex serve --port 3456
dendrite cortex sync --file web/src/Button.svelte --ticket DEN-810
dendrite cortex canvas dispatch --action WRITE_PLAN --prompt "Q4 roadmap"
```
