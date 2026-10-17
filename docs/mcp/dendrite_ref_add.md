---
id: dendrite_ref_add
title: "dendrite_ref_add"
sidebar_label: "dendrite_ref_add"
---

# dendrite_ref_add

Registers a new Tier 1 source reference repository for library reuse

## Input Schema

```json
{
  "properties": {
    "capabilities": {
      "description": "List of capability tags",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "id": {
      "description": "Unique reference ID (e.g. ref-custom)",
      "type": "string"
    },
    "language": {
      "description": "Primary language",
      "type": "string"
    },
    "license": {
      "description": "Open source or proprietary license",
      "type": "string"
    },
    "name": {
      "description": "Repository or library name",
      "type": "string"
    },
    "repository": {
      "description": "Git repository URL or local path",
      "type": "string"
    }
  },
  "required": [
    "id",
    "repository"
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
      "capabilities": [
        "item1"
      ],
      "id": "sample",
      "language": "sample",
      "license": "sample",
      "name": "sample",
      "repository": "sample"
    },
    "name": "dendrite_ref_add"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_ref_add '{"capabilities":["item1"],"id":"sample","language":"sample","license":"sample","name":"sample","repository":"sample"}'
```
