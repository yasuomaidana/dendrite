---
id: dendrite_commit
title: "dendrite_commit"
sidebar_label: "dendrite_commit"
---

# dendrite_commit

Autonomous Git commit engine with automatic local model delegation for conventional commit message synthesis and ticket provenance

## Input Schema

```json
{
  "properties": {
    "dry_run": {
      "description": "Simulate staging and message synthesis without modifying git history",
      "type": "boolean"
    },
    "files": {
      "description": "Specific file paths to stage (default: stage all)",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "message": {
      "description": "Explicit commit message. If omitted, synthesized via local model",
      "type": "string"
    },
    "model": {
      "description": "Local model name (default: qwen3.8:27b)",
      "type": "string"
    },
    "stage_all": {
      "description": "Stage all modified and untracked files (default: true)",
      "type": "boolean"
    },
    "ticket_id": {
      "description": "Target ticket ID (e.g. DEN-098)",
      "type": "string"
    },
    "use_local_model": {
      "description": "Delegate commit message synthesis to local model (default: true)",
      "type": "boolean"
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
      "dry_run": false,
      "files": [
        "item1"
      ],
      "message": "sample",
      "model": "qwen3.8:27b",
      "stage_all": false,
      "ticket_id": "DEN-985",
      "use_local_model": false
    },
    "name": "dendrite_commit"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_commit '{"dry_run":false,"files":["item1"],"message":"sample","model":"qwen3.8:27b","stage_all":false,"ticket_id":"DEN-985","use_local_model":false}'
```
