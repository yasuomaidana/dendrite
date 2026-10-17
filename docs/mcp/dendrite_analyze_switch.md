---
id: dendrite_analyze_switch
title: "dendrite_analyze_switch"
sidebar_label: "dendrite_analyze_switch"
---

# dendrite_analyze_switch

Toggle Quota Governor switches (allow_zero drain override past 20% floor, or operational analysis mode)

## Input Schema

```json
{
  "properties": {
    "allow_zero": {
      "description": "If true, allows draining quota down to 0%; if false, halts cloud sprint at 20% reserve floor",
      "type": "boolean"
    },
    "mode": {
      "description": "Switch operating mode: 'always', 'policy', or 'paused'",
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
      "allow_zero": false,
      "mode": "sample"
    },
    "name": "dendrite_analyze_switch"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_analyze_switch '{"allow_zero":false,"mode":"sample"}'
```
