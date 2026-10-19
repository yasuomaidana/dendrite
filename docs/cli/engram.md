---
id: engram
title: "dendrite engram"
sidebar_label: "engram"
---

# dendrite engram

Persistent segmented store, memory checkpointer, and chunk query

## Category

Soma & Axon Hillock

## Aliases

- `store`
- `engram-store`
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

### `dendrite engram chunks`

List chunk checkpoints

```bash
dendrite engram chunks [flags]
```

Aliases: `checkpoints`

| Flag | Description |
| :--- | :--- |
| `--project` | Only this project |

### `dendrite engram commit`

Commit a file as one segment of a chunk checkpoint

```bash
dendrite engram commit [flags]
```

| Flag | Description |
| :--- | :--- |
| `--chunk, -c` | Chunk ID |
| `--file, -f` | File to store |
| `--final` | Mark the chunk as complete |
| `--project` | Project the chunk belongs to |

### `dendrite engram purge`

Delete all segmented analysis data of a project

```bash
dendrite engram purge [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project to purge |

### `dendrite engram query`

List the committed segments of a project, including partial chunks

```bash
dendrite engram query [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project to query |

### `dendrite engram rm`

Delete a store's segments and blobs (needs --force or --yes)

```bash
dendrite engram rm <id> [flags]
```

Aliases: `remove`

| Flag | Description |
| :--- | :--- |
| `--force, -f` | Confirm the deletion |

### `dendrite engram status`

Summarize every stored project (segments, bytes, progress)

```bash
dendrite engram status [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Only this project |

## Examples

```bash
dendrite engram status
dendrite engram inspect dendrite --json
dendrite engram commit --project dendrite --chunk ast-1 --file graph.json --final
dendrite engram rm old-project --force
```
