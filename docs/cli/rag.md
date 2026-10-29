---
id: rag
title: "dendrite rag"
sidebar_label: "rag"
---

# dendrite rag

Code & documentation retrieval sidecar orchestration (dendrite-rag)

## Category

Cortex & Sensory Integration

## Aliases

- `retrieval`
- `retriever`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `context` | Generate a token-budgeted prompt context pack |
| `embedders` | List compiled and available embedding backends in dendrite-rag |
| `graph` | Query Leiden communities, god nodes, paths, and explanations |
| `graph communities` | List Leiden communities of files |
| `graph god-nodes` | Symbols with the most callers and type users |
| `grep` | Regex search over indexable repository files |
| `history` | Search Git commit messages and per-file diffs |
| `index` | Index project incrementally or from scratch with --full |
| `pull` | Fetch missing embeddings from shared tier for offline use |
| `purge` | Delete local RAG index for project |
| `push` | Upload local index to PostgreSQL shared tier |
| `search` | Hybrid search over the project index (or, with --scope synapse, every project of a synapse) |
| `settings` | Inspect the resolved .dendrite/rag.toml settings and env overrides |
| `show` | Display the full content of chunk IDs |
| `status` | Display RAG index metrics, coverage, and warnings |
| `symbols` | Find definitions or references of a symbol name |
| `sync` | Replay journaled offline commits to shared tier |

## Flags

None

## Subcommand Reference

### `dendrite rag context`

Generate a token-budgeted prompt context pack

```bash
dendrite rag context <query...> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--budget` | Estimated token budget (default: 2500) |
| `--corpus` | Corpus: all, code, docs, or tickets (the rendered Plane tickets) (default: all) |
| `--detail` | Detail level: auto, full, or map (default: auto) |
| `--docs-share` | Share of budget kept for documentation (default: 0.2) |
| `--project` | Project directory (default: .) |

### `dendrite rag graph`

Query Leiden communities, god nodes, paths, and explanations

```bash
dendrite rag graph [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project directory (default: .) |

### `dendrite rag grep`

Regex search over indexable repository files

```bash
dendrite rag grep <pattern> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--max` | Maximum matches (default: 200) |
| `--path` | Filter glob patterns (repeatable) |
| `--project` | Project directory (default: .) |

### `dendrite rag history`

Search Git commit messages and per-file diffs

```bash
dendrite rag history <query...> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project directory (default: .) |
| `--top-k, -k` | Maximum commits (default: 10) |

### `dendrite rag index`

Index project incrementally or from scratch with --full

```bash
dendrite rag index [flags]
```

| Flag | Description |
| :--- | :--- |
| `--embed-budget-ms` | Stop embedding after this many milliseconds |
| `--embed-budget-tokens` | Stop embedding after this many tokens |
| `--focus` | Under a budget, prioritize embedding chunks this question matches |
| `--full` | Force full re-indexing |
| `--project` | Project directory to index (default: .) |

### `dendrite rag pull`

Fetch missing embeddings from shared tier for offline use

```bash
dendrite rag pull [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project directory (default: .) |

### `dendrite rag purge`

Delete local RAG index for project

```bash
dendrite rag purge [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project directory (default: .) |

### `dendrite rag push`

Upload local index to PostgreSQL shared tier

```bash
dendrite rag push [flags]
```

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Simulate push without modifying shared tier |
| `--project` | Project directory (default: .) |

### `dendrite rag search`

Hybrid search over the project index (or, with --scope synapse, every project of a synapse)

```bash
dendrite rag search <query...> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--corpus` | Corpus filter: all, code, docs, or tickets (the rendered Plane tickets) (default: all) |
| `--min-score` | Minimum relevance score filter |
| `--project` | Project directory to search (default: .) |
| `--scope` | project (this directory) or synapse (every member of a synapse, merged by rank) (default: project) |
| `--synapse` | With --scope synapse: the synapse to search (default: the one containing this project) |
| `--top-k, -k` | Maximum number of hits (default: 10) |

### `dendrite rag settings`

Inspect the resolved .dendrite/rag.toml settings and env overrides

```bash
dendrite rag settings [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project directory (default: .) |

### `dendrite rag show`

Display the full content of chunk IDs

```bash
dendrite rag show <chunk_id...> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project directory (default: .) |

### `dendrite rag status`

Display RAG index metrics, coverage, and warnings

```bash
dendrite rag status [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project directory (default: .) |

### `dendrite rag symbols`

Find definitions or references of a symbol name

```bash
dendrite rag symbols <name> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project directory (default: .) |
| `--refs` | Find callers and users instead of definitions |

### `dendrite rag sync`

Replay journaled offline commits to shared tier

```bash
dendrite rag sync [flags]
```

| Flag | Description |
| :--- | :--- |
| `--project` | Project directory (default: .) |

## Examples

```bash
dendrite rag
```
