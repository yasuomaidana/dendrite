---
id: dendrite_graph_communities
title: "dendrite_graph_communities"
sidebar_label: "dendrite_graph_communities"
---

# dendrite_graph_communities

List Leiden communities of the project file graph, largest first, with top symbols and summary

## Input Schema

```json
{
  "properties": {
    "limit": {
      "description": "Maximum number of communities to return (default: 50)",
      "type": "integer"
    },
    "min_size": {
      "description": "Smallest community listed, in files (default: 2)",
      "type": "integer"
    },
    "project_path": {
      "description": "Target project directory to index and analyze (default: '.')",
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
      "limit": 1,
      "min_size": 1,
      "project_path": "'.'"
    },
    "name": "dendrite_graph_communities"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_graph_communities '{"limit":1,"min_size":1,"project_path":"'.'"}'
```
