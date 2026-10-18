---
id: motor
title: "dendrite motor"
sidebar_label: "motor"
---

# dendrite motor

Manage microservices (Plane, Penpot, Ollama, LiteLLM)

## Category

Postsynaptic Effectors & Actuation

## Aliases

- `service`
- `motor-unit`
- `effector-service`
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
