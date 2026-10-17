---
id: dendrite_story_fork
title: "dendrite_story_fork"
sidebar_label: "dendrite_story_fork"
---

# dendrite_story_fork

Fork a story into isolated Git worktrees and branches for competing implementation variants

## Input Schema

```json
{
  "properties": {
    "base_commit": {
      "description": "Optional base commit or branch to branch off (default: current HEAD)",
      "type": "string"
    },
    "story_id": {
      "description": "Identifier or slug of the story (e.g. 'DEN-064' or 'astar-search')",
      "type": "string"
    },
    "variants": {
      "description": "List of variant names to fork (at least 2 required, e.g. ['v1', 'v2'])",
      "items": {
        "type": "string"
      },
      "type": "array"
    }
  },
  "required": [
    "story_id",
    "variants"
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
      "base_commit": "current HEAD",
      "story_id": "sample",
      "variants": [
        "item1"
      ]
    },
    "name": "dendrite_story_fork"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_story_fork '{"base_commit":"current HEAD","story_id":"sample","variants":["item1"]}'
```
