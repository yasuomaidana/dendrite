---
id: dendrite_graph_explain
title: "dendrite_graph_explain"
sidebar_label: "dendrite_graph_explain"
---

# dendrite_graph_explain

Explain a symbol or file: callers, callees, definitions, imports, and community membership

## Input Schema

```json
{
  "properties": {
    "project_path": {
      "description": "Target project directory to index and analyze (default: '.')",
      "type": "string"
    },
    "target": {
      "description": "Symbol name or project-relative file path to explain",
      "type": "string"
    }
  },
  "required": [
    "target"
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
      "project_path": "'.'",
      "target": "sample"
    },
    "name": "dendrite_graph_explain"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_graph_explain '{"project_path":"'.'","target":"sample"}'
```
