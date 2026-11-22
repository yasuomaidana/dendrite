---
id: dendrite_rea_analyze
title: "dendrite_rea_analyze"
sidebar_label: "dendrite_rea_analyze"
---

# dendrite_rea_analyze

Analyze a Windows PE image: parse it, write its structure as documents and index them. Returns an evidence bundle under 4,000 tokens and an analysis id; read more with dendrite_rea_search instead of dumping the image again. A paused analysis is returned with paused=true.

## Input Schema

```json
{
  "properties": {
    "binary": {
      "description": "Path of the PE image",
      "type": "string"
    },
    "format": {
      "description": "pe32 or pe64 (default pe64)",
      "type": "string"
    },
    "id": {
      "description": "Name of the analysis (default: generated)",
      "type": "string"
    },
    "model": {
      "description": "Model to use on that tier",
      "type": "string"
    },
    "tier": {
      "description": "Model tier: local or paid",
      "type": "string"
    }
  },
  "required": [
    "binary"
  ],
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
      "binary": "sample",
      "format": "sample",
      "id": "generated",
      "model": "sample",
      "tier": "sample"
    },
    "name": "dendrite_rea_analyze"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_analyze '{"binary":"sample","format":"sample","id":"generated","model":"sample","tier":"sample"}'
```
