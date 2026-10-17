---
id: dendrite_analyze_status
title: "dendrite_analyze_status"
sidebar_label: "dendrite_analyze_status"
---

# dendrite_analyze_status

Query real-time status of the Token-Aware Hybrid Scheduler, quota governor, active execution tier, in-night/in-sprint window booleans, and cloud token spend

## Input Schema

```json
{
  "properties": {
    "remaining_quota": {
      "description": "Optional simulated or current remaining quota ratio (0.0 to 1.0)",
      "type": "number"
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
      "remaining_quota": 1
    },
    "name": "dendrite_analyze_status"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_analyze_status '{"remaining_quota":1}'
```
