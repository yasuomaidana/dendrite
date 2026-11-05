---
id: dendrite_inference_models
title: "dendrite_inference_models"
sidebar_label: "dendrite_inference_models"
---

# dendrite_inference_models

List the local models installed on Ollama and OxiLLaMa with size, loaded state and whether each fits this machine's memory budget (models marked too large get no gateway route and are refused at call time)

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
    "name": "dendrite_inference_models"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_inference_models
```
