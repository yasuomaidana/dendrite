---
id: dendrite_hitl_sync
title: "dendrite_hitl_sync"
sidebar_label: "dendrite_hitl_sync"
---

# dendrite_hitl_sync

Poll the GitHub verification issue of every Plane To Verify ticket: issues closed as completed move the ticket to Done, reviewer comments are reported as feedback.

## Input Schema

```json
{
  "properties": {
    "dry_run": {
      "description": "Report approvals without moving tickets to Done",
      "type": "boolean"
    },
    "repo": {
      "description": "Target GitHub repository in owner/repo format (default: the origin remote of the working directory)",
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
      "dry_run": false,
      "repo": "the origin remote of the working directory"
    },
    "name": "dendrite_hitl_sync"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_hitl_sync '{"dry_run":false,"repo":"the origin remote of the working directory"}'
```
