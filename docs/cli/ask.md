---
id: ask
title: "dendrite ask"
sidebar_label: "ask"
---

# dendrite ask

Ask questions to Dendrite about a project with local RAG and interactive paid fallback

## Category

Cortex & Sensory Integration

## Aliases

- `query-project`
- `rag-ask`
- `q`

## Subcommands

None

## Flags

| Flag | Description |
| :--- | :--- |
| `--hybrid` | Draft locally, then refine and verify with the paid model |
| `--local-model` | Preferred local model (default: detected from Ollama/OxiLLaMa) |
| `--paid` | Skip the local model and ask the paid model with the RAG context |
| `--paid-model` | Paid fallback model (default: gemini-2.5-pro) |
| `--project` | Project directory to index and search (default: .) |
| `--server` | Ask a central Dendrite RAG service instead (e.g. http://host:3456) |
| `--tier` | Shortcut for the tier: local, hybrid or paid |
| `--top-k, -k` | Retrieve the N best chunks instead of a token-budgeted context pack (default: context pack) |
| `--update-index` | Re-index changed project files before answering |

## Examples

```bash
dendrite ask "how do I check the available LLMs?"
dendrite ask --hybrid "how does the token gateway work?"
dendrite ask --server http://100.84.12.10:3456 "where is telemetry defined?"
dendrite ask --update-index --json "what does soma prepare write?"
```
