---
id: store
title: "dendrite store"
sidebar_label: "store"
---

# dendrite store

Persistent segmented store, memory checkpointer, and chunk query

## Category

Soma & Axon Hillock

## Aliases

- `engram-store`
- `engram`
- `gram`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `status` | Inspect persistent segmented store status and chunk count |
| `chunks` | List stored memory chunks and vector embeddings |
| `query` | Semantic similarity query over stored code memory |
| `commit` | Checkpoint active workspace state into persistent store |

## Flags

None

## Examples

```bash
dendrite store status
dendrite store chunks
```
