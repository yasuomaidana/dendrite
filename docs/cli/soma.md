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
| `diagnose` | Run environment pre-flight diagnostics (local Ollama, project status) |
| `install` | Install an agent (aider, openhands, copilot, agy) or a service (plane, litellm, ollama, ...) |
| `migrate-state` | Stop tracking .dendrite/ in every indexed repository (nothing is committed) |
| `plan` | Show the replication plan for this host |
| `prepare` | Sync agent rules and skills, and enforce CI/CD tooling defaults |
| `replicate` | Execute the replication plan (compose files, services, status) |
| `secrets` | Create .env from the published defaults and change them at install |
| `secrets ensure` | Set credentials once in ~/.dendrite/.env; the committed .env keeps the published defaults |
| `secrets status` | Show which credentials are the published default, custom or missing (never prints values) |
| `stacks` | Where the compose files and litellm_config.yaml live (~/.dendrite/stacks) |
| `stacks migrate` | Copy a checkout's stack files into ~/.dendrite/stacks (never overwrites) |
| `stacks path` | Print the stacks directory |
| `status` | Show real-time status of environment components |

## Flags

None

## Subcommand Reference

### `dendrite soma diagnose`

Run environment pre-flight diagnostics (local Ollama, project status)

```bash
dendrite soma diagnose
```

Aliases: `doctor`, `diag`

### `dendrite soma install`

Install an agent (aider, openhands, copilot, agy) or a service (plane, litellm, ollama, ...)

```bash
dendrite soma install <target> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--mode, -m` | Install mode: container, host or hybrid (default: container) |

```bash
dendrite soma install agy --yes
dendrite soma install litellm --mode host
```

### `dendrite soma migrate-state`

Stop tracking .dendrite/ in every indexed repository (nothing is committed)

```bash
dendrite soma migrate-state [flags]
```

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Show what would happen without changing anything |

```bash
dendrite soma migrate-state --dry-run
dendrite soma migrate-state
```

### `dendrite soma plan`

Show the replication plan for this host

```bash
dendrite soma plan [flags]
```

| Flag | Description |
| :--- | :--- |
| `--mode, -m` | Install mode: container, host or hybrid (default: container) |
| `--no-plane-ui` | Skip the Plane web UI |
| `--with-grafana` | Include Grafana |
| `--with-linear` | Include Linear |
| `--with-penpot` | Include Penpot |
| `--with-plane` | Include Plane |
| `--with-plane-ui` | Include the Plane web UI |

```bash
dendrite soma plan
dendrite soma plan --mode host --with-plane --with-grafana
```

### `dendrite soma prepare`

Sync agent rules and skills, and enforce CI/CD tooling defaults

```bash
dendrite soma prepare [path|project] [flags]
```

Aliases: `sync`, `update`, `configure`

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Show what would happen without changing anything |
| `--no-cicd` | Disable CI/CD and Copilot enforcement |
| `--no-skills` | Disable agent rules and skills synchronization |
| `--path` | Target project directory or registered project name (default: .) |
| `--tool` | Default developer and CI/CD assistant tool (default: copilot) |

```bash
dendrite soma prepare
dendrite soma prepare ../other-repo --dry-run
dendrite soma prepare --json
```

### `dendrite soma replicate`

Execute the replication plan (compose files, services, status)

```bash
dendrite soma replicate [flags]
```

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Show what would happen without changing anything |
| `--mode, -m` | Install mode: container, host or hybrid (default: container) |
| `--no-linear` | Skip Linear |
| `--no-plane-ui` | Skip the Plane web UI |
| `--with-plane` | Include Plane |
| `--with-plane-ui` | Include the Plane web UI |

```bash
dendrite soma replicate --dry-run
dendrite soma replicate --with-plane --no-plane-ui
```

### `dendrite soma secrets`

Create .env from the published defaults and change them at install

```bash
dendrite soma secrets
```

```bash
dendrite soma secrets ensure
dendrite soma secrets ensure --set POSTGRES_PASSWORD=mine --set LITELLM_MASTER_KEY=sk-mine
dendrite soma secrets ensure --generate
dendrite soma secrets status
```

### `dendrite soma secrets ensure`

Set credentials once in ~/.dendrite/.env; the committed .env keeps the published defaults

```bash
dendrite soma secrets ensure [flags]
```

| Flag | Description |
| :--- | :--- |
| `--generate` | Random values for every managed key still at its published default |
| `--local` | Write to this checkout's .env.local instead of the global file |
| `--rotate` | With --generate: replace custom values too |
| `--set` | Set KEY=VALUE (repeatable) |

### `dendrite soma stacks`

Where the compose files and litellm_config.yaml live (~/.dendrite/stacks)

```bash
dendrite soma stacks
```

```bash
dendrite soma stacks path
dendrite soma stacks migrate --dry-run
dendrite soma stacks migrate --up
```

### `dendrite soma stacks migrate`

Copy a checkout's stack files into ~/.dendrite/stacks (never overwrites)

```bash
dendrite soma stacks migrate [flags]
```

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Show what would happen without changing anything |
| `--from` | Source stacks folder (default: this checkout's stacks/) |
| `--up` | Then start the control stack from the new location (recreates containers whose mounts moved) |

## Examples

```bash
dendrite soma prepare --dry-run
dendrite soma plan --mode host --with-plane
dendrite soma install litellm
dendrite soma status
dendrite soma diagnose
```
