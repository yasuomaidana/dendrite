---
id: dendrite_vcs_guard
title: "dendrite_vcs_guard"
sidebar_label: "dendrite_vcs_guard"
---

# dendrite_vcs_guard

Check or enforce Git branch guardrails (prohibits direct work on main/master, guarantees develop default).

## Input Schema

```json
{
  "properties": {
    "mode": {
      "description": "Execution mode: 'STANDARD', 'DEGRADED_BUDGET', or 'AUDIT_CHANGES' (default: STANDARD)",
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
      "mode": "STANDARD"
    },
    "name": "dendrite_vcs_guard"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_vcs_guard '{"mode":"STANDARD"}'
```
