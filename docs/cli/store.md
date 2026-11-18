---
id: store
title: "dendrite store"
sidebar_label: "store"
---

# dendrite store

Persistent segmented store, checkpointer, and chunk query

## Category

Soma & Axon Hillock

## Aliases

- `segstore`
- `gram`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `chunks` | List chunk checkpoints |
| `commit` | Commit a file as one segment of a chunk checkpoint |
| `inspect` | Show one store's checkpoints and sample segments |
| `purge` | Delete all segmented analysis data of a project |
| `query` | List the committed segments of a project, including partial chunks |
| `rm` | Delete a store's segments and blobs (needs --force or --yes) |
| `status` | Summarize every stored project (segments, bytes, progress) |

## Flags

None

## Subcommand Reference

### `dendrite store chunks`

List chunk checkpoints

```bash
dendrite store chunks [flags]
```

Aliases: `checkpoints`

| Flag | Description |
| :--- | :--- |
| `--project` | Only this project |

### `dendrite store commit`

Commit a file as one segment of a chunk checkpoint

```bash
dendrite store commit [flags]
```

| Flag | Description |
| :--- | :--- |
| `--chunk, -c` | Chunk ID |
| `--file, -f` | File to store |
| `--final` | Mark the chunk as complete |
| `--project` | Project the chunk belongs to |

### `dendrite store purge`

Delete all segmented analysis data of a project

```bash
dendrite store purge [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project to purge |

### `dendrite store query`

List the committed segments of a project, including partial chunks

```bash
dendrite store query [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project to query |

### `dendrite store rm`

Delete a store's segments and blobs (needs --force or --yes)

```bash
dendrite store rm <id> [flags]
```

Aliases: `remove`

| Flag | Description |
| :--- | :--- |
| `--force, -f` | Confirm the deletion |

### `dendrite store status`

Summarize every stored project (segments, bytes, progress)

```bash
dendrite store status [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Only this project |

## Examples

```bash
dendrite store status
dendrite store inspect dendrite --json
dendrite store commit --project dendrite --chunk ast-1 --file graph.json --final
dendrite store rm old-project --force
```
