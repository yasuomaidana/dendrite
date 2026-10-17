---
id: dendrite_analyze_policy
title: "dendrite_analyze_policy"
sidebar_label: "dendrite_analyze_policy"
---

# dendrite_analyze_policy

Inspect or configure the Token-Aware Hybrid Scheduler policy (night local window, weekly quota reset, 48h pre-reset cloud sprint)

## Input Schema

```json
{
  "properties": {
    "allow_zero": {
      "description": "Override to permit draining quota down to 0% (bypasses safety reserve floor)",
      "type": "boolean"
    },
    "mode": {
      "description": "Operating mode: 'auto', 'always', 'night', 'sprint', or 'paused'",
      "type": "string"
    },
    "night_window": {
      "description": "Scheduled zero-cost night window in 'HH:MM-HH:MM' format (e.g. '23:00-06:00')",
      "type": "string"
    },
    "reserve_floor": {
      "description": "Safety reserve quota floor (0.0 to 1.0, default 0.20 for 20% reserve)",
      "type": "number"
    },
    "sprint_before_reset": {
      "description": "Duration before weekly reset to activate cloud sprint (e.g. '48h')",
      "type": "string"
    },
    "weekly_reset": {
      "description": "Weekly quota reset time in 'DAY HH:MM' format (e.g. 'SUN 23:59')",
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
      "mode": "sample",
      "night_window": "sample",
      "reserve_floor": 1,
      "sprint_before_reset": "sample",
      "weekly_reset": "sample"
    },
    "name": "dendrite_analyze_policy"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_analyze_policy '{"allow_zero":false,"mode":"sample","night_window":"sample","reserve_floor":1,"sprint_before_reset":"sample","weekly_reset":"sample"}'
```
