---
id: dendrite_story_benchmark
title: "dendrite_story_benchmark"
sidebar_label: "dendrite_story_benchmark"
---

# dendrite_story_benchmark

Run comparative differential performance benchmarks (TTFT, TPS, memory, CPU) across variant worktrees

## Input Schema

```json
{
  "properties": {
    "iterations": {
      "description": "Number of benchmark iterations per variant (default: 100)",
      "type": "integer"
    },
    "story_id": {
      "description": "Target story identifier",
      "type": "string"
    },
    "virtual_users": {
      "description": "Number of simulated concurrent users (default: 1)",
      "type": "integer"
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
      "iterations": 1,
      "story_id": "sample",
      "virtual_users": 1
    },
    "name": "dendrite_story_benchmark"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_story_benchmark '{"iterations":1,"story_id":"sample","virtual_users":1}'
```
