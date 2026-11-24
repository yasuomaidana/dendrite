---
id: dendrite_rea_adr
title: "dendrite_rea_adr"
sidebar_label: "dendrite_rea_adr"
---

# dendrite_rea_adr

Draft a proposed Architecture Decision Record from a finished analysis (next free ADR number; the decision and consequences are left for a person).

## Input Schema

```json
{
  "properties": {
    "id": {
      "description": "Analysis id, as printed by dendrite_rea_analyze",
      "type": "string"
    },
    "output": {
      "description": "Directory of decision records (default docs/architecture/decisions)",
      "type": "string"
    },
    "title": {
      "description": "Title of the record",
      "type": "string"
    }
  },
  "required": [
    "id"
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
      "id": "sample",
      "output": "sample",
      "title": "sample"
    },
    "name": "dendrite_rea_adr"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_adr '{"id":"sample","output":"sample","title":"sample"}'
```
