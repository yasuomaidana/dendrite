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
