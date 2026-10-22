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
| `--corpus` | Corpora searched: code (default), all (code and documentation) or docs (documentation only, for design and conceptual questions) |
| `--docs, -d` | Also search the documentation (default: code only); same as --corpus all |
| `--docs-share` | Share of the context kept for documentation with --corpus all, 0 to 1 (default: [ask] docs_share in .dendrite/rag.toml or DENDRITE_RAG_ASK_DOCS_SHARE, else 0.2); implies --corpus all |
| `--hybrid` | Draft locally, then refine and verify with the paid model |
| `--local-model` | Local model on the LiteLLM gateway (default: [ask] local_model in .dendrite/rag.toml, else the gateway's local model on the most preferred reachable GPU engine; never OxiLLaMa, which prefills on the CPU) |
| `--paid` | Skip the local model and ask the paid model with the RAG context |
| `--paid-model` | Paid fallback model (default: gemini-2.5-pro) |
| `--project` | Project directory to index and search (default: .) |
| `--server` | Ask a central Dendrite RAG service instead (e.g. http://host:3456) |
| `--tier` | Shortcut for the tier: local, hybrid or paid |
| `--top-k, -k` | Retrieve the N best chunks instead of a token-budgeted context pack |
| `--update-index` | Rebuild the project index and ask the model again instead of reusing a cached answer |

## Examples

```bash
dendrite ask "how do I check the available LLMs?"
dendrite ask --hybrid "how does the token gateway work?"
dendrite ask --server http://100.84.12.10:3456 "where is telemetry defined?"
dendrite ask -d --update-index --json "what does soma prepare write?"
dendrite ask -d "how do I check the available LLMs?"
dendrite ask --corpus docs "what is the design of the session handoff?"
dendrite ask -d --docs-share 0.5 "how do agents start a session?"
```
