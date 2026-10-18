---
id: synapse
title: "dendrite synapse"
sidebar_label: "synapse"
---

# dendrite synapse

Typed cross-project links with scoped MCP servers and self-healing paths

## Category

Synaptic Cleft & Gating

## Aliases

- `syn`
- `synapses`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `create` | Create a synapse manifest linking member projects |
| `heal` | Find moved member projects and repair their paths |
| `ide-sync` | Write IDE MCP configuration for a synapse's members |
| `inspect` | Show members, contracts and parity commands of a synapse |
| `list` | List synapses and their health |
| `locate` | Locate a member project on disk |
| `move` | Move a member project and update every manifest atomically |

## Flags

None

## Subcommand Reference

### `dendrite synapse create`

Create a synapse manifest linking member projects

```bash
dendrite synapse create <id> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--contract` | Contract kind:producer:path[>consumer:path...] (repeatable) |
| `--members` | Members as <dir>[=alias][:role],... |
| `--name` | Display name (default: id) |
| `--parity` | Parity command alias=cmd (repeatable) |
| `--sync` | Sync strategy |

```bash
dendrite synapse create billing --members ../api=api:producer,../web=web:consumer --contract openapi:api:spec.yaml>web:client.ts
```

### `dendrite synapse heal`

Find moved member projects and repair their paths

```bash
dendrite synapse heal [flags]
```

| Flag | Description |
| :--- | :--- |
| `--auto` | Apply the repairs instead of only reporting them |
| `--depth` | Maximum search depth |
| `--roots` | Search roots (comma-separated, repeatable) |

```bash
dendrite synapse heal --roots ~/code,~/work --depth 3 --auto
```

### `dendrite synapse ide-sync`

Write IDE MCP configuration for a synapse's members

```bash
dendrite synapse ide-sync <id> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--binary` | dendrite binary for the MCP entry (default: this executable) |

### `dendrite synapse list`

List synapses and their health

```bash
dendrite synapse list
```

Aliases: `ls`

### `dendrite synapse move`

Move a member project and update every manifest atomically

```bash
dendrite synapse move <alias|project-id> <new-path>
```

Aliases: `mv`

## Examples

```bash
dendrite synapse create billing --members ../api=api:producer,../web=web:consumer
dendrite synapse list --json
dendrite synapse heal --auto
```
