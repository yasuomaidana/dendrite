---
id: inhibitory-triage
title: "dendrite inhibitory-triage"
sidebar_label: "inhibitory-triage"
---

# dendrite inhibitory-triage

Resolve local agent task triage and priority routing

## Category

Postsynaptic Effectors & Actuation

## Aliases

- `triage`
- `trg`

## Subcommands

None

## Flags

| Flag | Description |
| :--- | :--- |
| `--engine, -e` | Local engine: ollama, ollama-validated, huggingface, candle or oxillama |
| `--file, -f` | Active file whose content is triaged with the prompt |
| `--local, -l` | Execute on the local engine when the tier allows it |
| `--rag, -r` | Retrieve workspace context (RAG) |
| `--tier` | Force a tier: local, hybrid or cloud |

## Examples

```bash
dendrite inhibitory-triage "rename this variable" --file main.go
dendrite triage "design a sharded cache" --tier cloud
dendrite triage "summarize the docs" --rag --local
```
