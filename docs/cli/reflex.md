---
id: reflex
title: "dendrite reflex"
sidebar_label: "reflex"
---

# dendrite reflex

Table-driven test synthesis, Testcontainers & UI tests

## Category

Postsynaptic Effectors & Actuation

## Aliases

- `test`
- `myotatic-reflex`
- `tst`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `auto` | Synthesize table-driven tests for a source file with a self-healing loop |
| `ui` | Run the web UI browser journeys and a WebSocket telemetry burst test |

## Flags

None

## Subcommand Reference

### `dendrite reflex auto`

Synthesize table-driven tests for a source file with a self-healing loop

```bash
dendrite reflex auto [file] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--file, -f` | Go source file to cover |
| `--with-testcontainers` | Generate Testcontainers fixtures for detected dependencies |

### `dendrite reflex ui`

Run the web UI browser journeys and a WebSocket telemetry burst test

```bash
dendrite reflex ui [flags]
```

| Flag | Description |
| :--- | :--- |
| `--base-url` | Cockpit URL (see 'cortex serve') (default: http://127.0.0.1:3456) |
| `--no-headless` | Show the browser |

## Examples

```bash
dendrite reflex auto pkg/env/detector.go
dendrite reflex auto --file pkg/env/detector.go --with-testcontainers
dendrite reflex ui --base-url http://127.0.0.1:3456
```
