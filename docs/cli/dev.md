---
id: dev
title: "dendrite dev"
sidebar_label: "dev"
---

# dendrite dev

Developer diagnostics, local Ollama probe, and ticket tools

## Category

Soma & Axon Hillock

## Aliases

None

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `ollama` | Probe local Ollama inference engine health, latency, and models |
| `ticket` | List, inspect, or transition tickets in DEV_STATUS.md |
| `topology` | Inspect service placement across compute nodes |
| `gateway` | Render dynamic LiteLLM configuration YAML |
| `audit-prompt` | Analyze prompt text length and estimated token volume |

## Flags

None

## Examples

```bash
dendrite dev ollama probe
dendrite dev ticket list
dendrite dev audit-prompt "Explain Dendrite architecture"
```
