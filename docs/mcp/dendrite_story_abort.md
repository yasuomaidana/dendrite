---
id: dendrite_story_abort
title: "dendrite_story_abort"
sidebar_label: "dendrite_story_abort"
---

# dendrite_story_abort

Safely abort and purge all speculative worktrees and branches for a story without merging

## Input Schema

```json
{
  "properties": {
    "story_id": {
      "description": "Target story identifier to clean up",
      "type": "string"
    }
  },
  "required": [
    "story_id"
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
      "story_id": "sample"
    },
    "name": "dendrite_story_abort"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_story_abort '{"story_id":"sample"}'
```
