---
id: greeting
title: "dendrite greeting"
sidebar_label: "greeting"
---

# dendrite greeting

Generate dynamic 10-style greeting with cliché suppression

## Category

Synaptic Cleft & Gating

## Aliases

- `greet`
- `pcontext-greeting`

## Subcommands

None

## Flags

| Flag | Description |
| :--- | :--- |
| `--topic` | User's current topic or project |
| `--decision` | Decision point being weighed |
| `--milestone` | Milestone achieved or in-flight |
| `--json` | Output greeting result in JSON format |

## Examples

```bash
dendrite greeting --topic "DEN-985"
dendrite greeting --json
```
