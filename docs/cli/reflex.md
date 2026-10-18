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
| `auto` | Synthesize table-driven tests with Testcontainers & self-healing |
| `ui` | Run Web UI browser journeys & WebSocket burst load tests |

## Flags

| Flag | Description |
| :--- | :--- |
| `--file` | Target Go source file for table-driven test synthesis |
| `--base-url` | Base target URL for UI browser test journeys |

## Examples

```bash
dendrite test auto --file ./pkg/env/detector.go
dendrite test ui --base-url http://localhost:3456
```
