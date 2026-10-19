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
| `health` | Check whether a service is listening (default penpot) |
| `list` | List the service catalog |
| `restart` | Restart a service's containers (default penpot) |
| `start` | Start a service's compose stack and check its port (default penpot) |
| `stop` | Stop a service's containers (default penpot) |

## Flags

None

## Subcommand Reference

### `dendrite motor health`

Check whether a service is listening (default penpot)

```bash
dendrite motor health [service] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--deep` | Also list the service's containers |

### `dendrite motor list`

List the service catalog

```bash
dendrite motor list
```

Aliases: `ls`

### `dendrite motor start`

Start a service's compose stack and check its port (default penpot)

```bash
dendrite motor start [service] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--target` | Node to start on (default: local) |
| `--verify-mcp` | For penpot, also check the Penpot MCP bridge |

## Examples

```bash
dendrite motor list
dendrite motor start plane
dendrite motor health litellm --deep
dendrite motor stop penpot
```
