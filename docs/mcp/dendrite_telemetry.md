---
id: dendrite_telemetry
title: "dendrite_telemetry"
sidebar_label: "dendrite_telemetry"
---

# dendrite_telemetry

Sample real-time SoC thermal metrics and 24h/7d token spend totals

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
    "name": "dendrite_telemetry"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_telemetry
```
