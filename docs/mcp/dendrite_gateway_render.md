---
id: dendrite_gateway_render
title: "dendrite_gateway_render"
sidebar_label: "dendrite_gateway_render"
---

# dendrite_gateway_render

Render dynamic LiteLLM YAML routing configuration for a given topology scenario

## Input Schema

```json
{
  "properties": {
    "scenario": {
      "description": "Topology scenario: 'local-hybrid', 'cloud-burst', 'full-local'",
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
      "scenario": "sample"
    },
    "name": "dendrite_gateway_render"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_gateway_render '{"scenario":"sample"}'
```
