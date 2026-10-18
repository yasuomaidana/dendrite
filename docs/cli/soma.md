---
id: soma
title: "dendrite soma"
sidebar_label: "soma"
---

# dendrite soma

Manage and replicate development environments

## Category

Soma & Axon Hillock

## Aliases

- `env`
- `homeostasis`
- `soma-homeostasis`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `doctor` | Inspect target computer OS, package manager and installed tools |
| `plan` | Generate environment replication plan (--mode container|host|hybrid) |
| `replicate` | Replicate development environment (Docker containers or host tools) |
| `install` | Install tool directly on host computer or spin up container |
| `status` | Query real-time health and status of replicated components |
| `prepare` | Synchronize project environment, agent rules, and CI/CD tooling |

## Flags

| Flag | Description |
| :--- | :--- |
| `--mode` | Replication execution mode: 'container', 'host', or 'hybrid' (default: container) |
| `--with-linear` | Include Linear issue tracker container |
| `--with-plane` | Include Plane v2 project management container |
| `--with-penpot` | Include Penpot design system container |
| `--with-grafana` | Include Prometheus & Grafana observability suite |
| `--dry-run` | Simulate actions without modifying local disk or services |

## Examples

```bash
dendrite env doctor
dendrite env plan --mode hybrid --with-plane --with-penpot
dendrite env replicate --mode hybrid
dendrite env status
```
