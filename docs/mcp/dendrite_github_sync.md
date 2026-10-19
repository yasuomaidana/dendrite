---
id: dendrite_github_sync
title: "dendrite_github_sync"
sidebar_label: "dendrite_github_sync"
---

# dendrite_github_sync

Mirror Plane verification state onto GitHub issues: open a HITL verification issue for every To Verify ticket, add missing labels, and close issues of tickets Done in Plane. Tickets come from Plane and the repository from the git origin remote.

## Input Schema

```json
{
  "properties": {
    "dry_run": {
      "description": "Plan the GitHub changes without applying them",
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
    "name": "dendrite_github_sync"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_github_sync '{"dry_run":false,"repo":"the origin remote of the working directory"}'
```
