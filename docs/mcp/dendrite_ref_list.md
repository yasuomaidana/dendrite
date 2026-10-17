---
id: dendrite_ref_list
title: "dendrite_ref_list"
sidebar_label: "dendrite_ref_list"
---

# dendrite_ref_list

Lists registered Tier 1 source references for library reuse

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
    "name": "dendrite_ref_list"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ref_list
```
