---
id: dendrite_pool_list
title: "dendrite_pool_list"
sidebar_label: "dendrite_pool_list"
---

# dendrite_pool_list

List all registered project members in the multi-project pool

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
    "name": "dendrite_pool_list"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_pool_list
```
