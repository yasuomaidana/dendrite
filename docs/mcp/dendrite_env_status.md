---
id: dendrite_env_status
title: "dendrite_env_status"
sidebar_label: "dendrite_env_status"
---

# dendrite_env_status

Query real-time health and status of replicated environment components (Docker, Plane, LiteLLM, Ollama)

## Input Schema

```json
{
  "properties": {},
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
    "arguments": {},
    "name": "dendrite_env_status"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_env_status
```
