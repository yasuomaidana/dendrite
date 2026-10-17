---
id: service
title: "dendrite service"
sidebar_label: "service"
---

# dendrite service

Manage microservices (Plane, Penpot, Ollama, LiteLLM)

## Category

Postsynaptic Effectors & Actuation

## Aliases

- `motor-unit`
- `effector-service`
- `motor`
- `svc`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `start` | Start specified microservice |
| `stop` | Stop specified microservice |
| `restart` | Restart specified microservice |
| `health` | Check health of specified microservice |
| `list` | List all managed microservices and status |
| `ui-sync` | Synchronize service state with Web UI |

## Flags

None

## Examples

```bash
dendrite service list
dendrite service health plane
```
