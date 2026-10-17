---
id: dendrite_tokens_status
title: "dendrite_tokens_status"
sidebar_label: "dendrite_tokens_status"
---

# dendrite_tokens_status

Inspect remaining tokens in the day window and week window, and confirm Google Antigravity (AGY) connectivity and availability

## Input Schema

```json
{
  "properties": {
    "model": {
      "description": "Target model identifier (default: \"gemini-3.8-flash-high\")",
      "type": "string"
    },
    "policy_file": {
      "description": "Optional path to scheduler_policy.json override",
      "type": "string"
    },
    "probe": {
      "description": "If true, executes an active probe to confirm AGY endpoint responsiveness",
      "type": "boolean"
    },
    "spend_file": {
      "description": "Optional path to spend.json override",
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
      "model": "gemini-3.8-flash-high",
      "policy_file": "DEV_STATUS.md",
      "probe": false,
      "spend_file": "DEV_STATUS.md"
    },
    "name": "dendrite_tokens_status"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_tokens_status '{"model":"gemini-3.8-flash-high","policy_file":"DEV_STATUS.md","probe":false,"spend_file":"DEV_STATUS.md"}'
```
