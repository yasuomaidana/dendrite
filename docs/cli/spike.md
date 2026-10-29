---
id: spike
title: "dendrite spike"
sidebar_label: "spike"
---

# dendrite spike

Unified multi-provider inference engine gateway (Ollama, OxiLLaMa, vLLM, TGI)

## Category

Synaptic Cleft & Gating

## Aliases

- `inference`
- `action-potential`
- `inf`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `hold` | Start OxiLLaMa if needed and keep it alive while this command runs (--for, or until Ctrl+C) |
| `list` | List registered engines, their models and the failover cascade |
| `migrate` | Copy a model's blobs between engines (default Ollama to OxiLLaMa, which runs prompts on the CPU), optionally evicting the source |
| `models` | List the local models installed on Ollama and OxiLLaMa and whether each fits this machine's memory |
| `probe` | Check engine reachability, latency and models (all engines, one engine, or a raw Ollama endpoint) |
| `render` | Render the registered engines' model routes as LiteLLM config |
| `serve` | Run the OxiLLaMa OpenAI-compatible server in the foreground (Ctrl+C stops it) |
| `status` | Show whether the OxiLLaMa server runs, who started it and which sessions hold it |
| `stop` | Stop a running OxiLLaMa server and free its port |
| `switch` | Move an engine to the front of the failover cascade |
| `zero-cost` | Manage the zero-cost (self-hosted) model and prefix rules |
| `zero-cost add` | Register a zero-cost model or prefix rule |
| `zero-cost list` | List zero-cost models and prefixes |
| `zero-cost remove` | Unregister a zero-cost model or prefix rule |
| `zero-cost reset` | Restore the default rules plus the active .env configuration |
| `zero-cost test` | Check whether a model qualifies as zero-cost |

## Flags

None

## Subcommand Reference

### `dendrite spike hold`

Start OxiLLaMa if needed and keep it alive while this command runs (--for, or until Ctrl+C)

```bash
dendrite spike hold [flags]
```

| Flag | Description |
| :--- | :--- |
| `--for` | hold for this long (default: until interrupted) |
| `--port` | OxiLLaMa server port (default: 8080) |

### `dendrite spike list`

List registered engines, their models and the failover cascade

```bash
dendrite spike list
```

Aliases: `ls`

### `dendrite spike migrate`

Copy a model's blobs between engines (default Ollama to OxiLLaMa, which runs prompts on the CPU), optionally evicting the source

```bash
dendrite spike migrate <model> [flags]
```

Aliases: `move`

| Flag | Description |
| :--- | :--- |
| `--delete-source` | Evict the model from the source after copying |
| `--dry-run` | Show what would happen without changing anything |
| `--from` | Source engine (default: ollama) |
| `--to` | Target engine (default: oxillama) |

### `dendrite spike models`

List the local models installed on Ollama and OxiLLaMa and whether each fits this machine's memory

```bash
dendrite spike models [flags]
```

| Flag | Description |
| :--- | :--- |
| `--ollama` | Ollama endpoint (default http://127.0.0.1:11434) |
| `--oxillama` | OxiLLaMa endpoint (default http://127.0.0.1:8080) |

```bash
dendrite spike models
dendrite spike models --json
```

### `dendrite spike probe`

Check engine reachability, latency and models (all engines, one engine, or a raw Ollama endpoint)

```bash
dendrite spike probe [engine-id] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--endpoint` | Probe an Ollama-compatible endpoint URL instead of a registered engine |

### `dendrite spike serve`

Run the OxiLLaMa OpenAI-compatible server in the foreground (Ctrl+C stops it)

```bash
dendrite spike serve [flags]
```

Aliases: `start`

| Flag | Description |
| :--- | :--- |
| `--port` | OxiLLaMa server port (default: 8080) |

### `dendrite spike status`

Show whether the OxiLLaMa server runs, who started it and which sessions hold it

```bash
dendrite spike status [flags]
```

| Flag | Description |
| :--- | :--- |
| `--port` | OxiLLaMa server port (default: 8080) |

### `dendrite spike stop`

Stop a running OxiLLaMa server and free its port

```bash
dendrite spike stop [flags]
```

Aliases: `kill`

| Flag | Description |
| :--- | :--- |
| `--port` | OxiLLaMa server port (default: 8080) |

### `dendrite spike zero-cost`

Manage the zero-cost (self-hosted) model and prefix rules

```bash
dendrite spike zero-cost
```

Aliases: `zerocost`

```bash
dendrite spike zero-cost list
dendrite spike zero-cost add local/ --prefix
dendrite spike zero-cost test qwen2.5-coder:14b
```

### `dendrite spike zero-cost add`

Register a zero-cost model or prefix rule

```bash
dendrite spike zero-cost add <model|prefix/> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--env, -e` | Also write the rules to .env (always done when .env exists) |
| `--prefix, -p` | Treat the pattern as a prefix rule |

### `dendrite spike zero-cost list`

List zero-cost models and prefixes

```bash
dendrite spike zero-cost list
```

Aliases: `ls`

### `dendrite spike zero-cost remove`

Unregister a zero-cost model or prefix rule

```bash
dendrite spike zero-cost remove <model|prefix/> [flags]
```

Aliases: `rm`, `delete`

| Flag | Description |
| :--- | :--- |
| `--env, -e` | Also write the rules to .env (always done when .env exists) |

### `dendrite spike zero-cost test`

Check whether a model qualifies as zero-cost

```bash
dendrite spike zero-cost test <model>
```

Aliases: `check`

## Examples

```bash
dendrite spike list
dendrite spike models
dendrite spike probe ollama-metal
dendrite spike probe --endpoint http://10.0.0.5:11434
dendrite spike migrate qwen2.5-coder:7b --to oxillama --delete-source
dendrite spike zero-cost add my-model --env
```
