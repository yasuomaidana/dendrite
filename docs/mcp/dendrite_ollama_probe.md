---
id: dendrite_ollama_probe
title: "dendrite_ollama_probe"
sidebar_label: "dendrite_ollama_probe"
---

# dendrite_ollama_probe

Probe local Ollama inference engine for latency and available local models (qwen2.5-coder, llama3.2, hermes3)

## Input Schema

```json
{
  "properties": {
    "url": {
      "description": "Ollama base URL (default: http://127.0.0.1:11434)",
      "type": "string"
    }
  },
  "type": "object"
}
```

## Example Call

### JSON-RPC

```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "arguments": {
      "url": "http://127.0.0.1:11434"
    },
    "name": "dendrite_ollama_probe"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ollama_probe '{"url":"http://127.0.0.1:11434"}'
```
