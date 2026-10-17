---
id: dendrite_github_sync
title: "dendrite_github_sync"
sidebar_label: "dendrite_github_sync"
---

# dendrite_github_sync

Synchronize prioritized P0 tickets, dependency relations, and governance labels with remote GitHub Issues (yasuomaidana/dendrite).

## Input Schema

```json
{
  "properties": {
    "repo": {
      "description": "Target GitHub repository in owner/repo format (default: yasuomaidana/dendrite)",
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
      "repo": "yasuomaidana/dendrite"
    },
    "name": "dendrite_github_sync"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_github_sync '{"repo":"yasuomaidana/dendrite"}'
```
