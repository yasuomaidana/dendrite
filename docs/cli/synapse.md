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
| `channel` | Show or set the branch members track by default (stable: main, dev: develop) |
| `create` | Create a synapse manifest linking member projects |
| `heal` | Find moved member projects and repair their paths |
| `ide-sync` | Write IDE MCP configuration for a synapse's members |
| `inspect` | Show members, contracts and parity commands of a synapse |
| `list` | List synapses and their health |
| `locate` | Locate a member project on disk |
| `move` | Move a member project and update every manifest atomically |
| `set-plane` | Bind a member to its Plane project identifier (- clears it) |
| `set-source` | Consume a member from its local checkout or its remote, optionally tracking another ref |
| `status` | Fetch every member and report ahead/behind, dirty files and drift against its tracked ref |
| `sync` | Fast-forward members to their tracked ref, lock their commits and relink (go.work / cargo patches) |

## Flags

None

## Subcommand Reference

### `dendrite synapse channel`

Show or set the branch members track by default (stable: main, dev: develop)

```bash
dendrite synapse channel <id> [stable|dev]
```

```bash
dendrite synapse channel dendrite-ecosystem
dendrite synapse channel dendrite-ecosystem dev
```

### `dendrite synapse create`

Create a synapse manifest linking member projects

```bash
dendrite synapse create <id> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--contract` | Contract kind:producer:path[>consumer:path...] (repeatable) |
| `--members` | Members as <dir>[=alias][:role][@PLANE],... |
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

### `dendrite synapse set-plane`

Bind a member to its Plane project identifier (- clears it)

```bash
dendrite synapse set-plane <id> <alias> <PLANE-IDENT|->
```

```bash
dendrite synapse set-plane dendrite-ecosystem git-rs DGIT
```

### `dendrite synapse set-source`

Consume a member from its local checkout or its remote, optionally tracking another ref

```bash
dendrite synapse set-source <id> <alias> local|remote [flags]
```

| Flag | Description |
| :--- | :--- |
| `--module` | Go module path or Cargo crate name consumers link against |
| `--ref` | Branch, tag or commit to track (empty: the channel's branch) |
| `--remote` | Git URL of the member (default: its origin) |

```bash
dendrite synapse set-source dendrite-ecosystem rag-rs local --ref feat/ann
dendrite synapse set-source dendrite-ecosystem mcp-hub remote --ref develop
```

### `dendrite synapse status`

Fetch every member and report ahead/behind, dirty files and drift against its tracked ref

```bash
dendrite synapse status <id> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--no-fetch` | Use the last fetched remote state |

```bash
dendrite synapse status dendrite-ecosystem
dendrite synapse status dendrite-ecosystem --no-fetch --json
```

### `dendrite synapse sync`

Fast-forward members to their tracked ref, lock their commits and relink (go.work / cargo patches)

```bash
dendrite synapse sync <id> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--autostash` | Stash uncommitted changes around member fast-forwards |
| `--enforce` | Switch clean members on another branch to their tracked branch |
| `--merge` | Merge <alias>=<branch> into that member after updating it (repeatable) |
| `--no-fetch` | Use the last fetched remote state |
| `--no-link` | Do not apply the sync strategy |
| `--rebase` | Replay local commits with rebase instead of fast-forward only |

```bash
dendrite synapse sync dendrite-ecosystem
dendrite synapse sync dendrite-ecosystem --enforce
dendrite synapse sync dendrite-ecosystem --merge mcp-hub=feat/x
```

## Examples

```bash
dendrite synapse create billing --members ../api=api:producer,../web=web:consumer
dendrite synapse list --json
dendrite synapse heal --auto
```
