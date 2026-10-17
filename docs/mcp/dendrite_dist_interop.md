---
id: dendrite_dist_interop
title: "dendrite_dist_interop"
sidebar_label: "dendrite_dist_interop"
---

# dendrite_dist_interop

Coordinate cross-project dependencies across owned repositories (e.g. Docula -> Oxipix), generate unblocking fallbacks and reconcile

## Input Schema

```json
{
  "properties": {
    "action": {
      "description": "Interop coordination action",
      "enum": [
        "coordinate",
        "complete",
        "reconcile",
        "list"
      ],
      "type": "string"
    },
    "allow_fallback": {
      "description": "Whether to unblock requester immediately with interim fallback and TODO",
      "type": "boolean"
    },
    "coordination_id": {
      "description": "Coordination record ID (for complete/reconcile)",
      "type": "string"
    },
    "dependency_project": {
      "description": "Project supplying capability (e.g. Oxipix)",
      "type": "string"
    },
    "feature_requested": {
      "description": "Feature name or signature to add",
      "type": "string"
    },
    "requester_project": {
      "description": "Project requesting capability (e.g. Docula)",
      "type": "string"
    }
  },
  "required": [
    "action"
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
      "action": "sample",
      "allow_fallback": false,
      "coordination_id": "sample",
      "dependency_project": "sample",
      "feature_requested": "sample",
      "requester_project": "sample"
    },
    "name": "dendrite_dist_interop"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_dist_interop '{"action":"sample","allow_fallback":false,"coordination_id":"sample","dependency_project":"sample","feature_requested":"sample","requester_project":"sample"}'
```
