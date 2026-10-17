---
id: dendrite_hitl_sync
title: "dendrite_hitl_sync"
sidebar_label: "dendrite_hitl_sync"
---

# dendrite_hitl_sync

Poll remote GitHub Issues for Human-in-the-Loop review approvals or change feedback.

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
    "name": "dendrite_hitl_sync"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_hitl_sync
```
