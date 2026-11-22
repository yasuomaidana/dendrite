---
id: dendrite_rea_export
title: "dendrite_rea_export"
sidebar_label: "dendrite_rea_export"
---

# dendrite_rea_export

Write a finished analysis of any format (PE, ELF, Mach-O, package, source tree, ...) as a data module in rust, go, python or typescript under output: sections, libraries, exports, files, notes and the functions an engine found.

## Input Schema

```json
{
  "properties": {
    "id": {
      "description": "Analysis id, as printed by dendrite_rea_analyze",
      "type": "string"
    },
    "language": {
      "description": "rust (default), go, python or typescript",
      "type": "string"
    },
    "output": {
      "description": "Directory to write \u003cid\u003e.\u003cext\u003e into",
      "type": "string"
    }
  },
  "required": [
    "id",
    "output"
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
      "language": "sample",
      "output": "sample"
    },
    "name": "dendrite_rea_export"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rea_export '{"id":"sample","language":"sample","output":"sample"}'
```
