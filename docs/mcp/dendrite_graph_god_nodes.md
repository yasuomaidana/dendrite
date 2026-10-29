---
id: dendrite_graph_god_nodes
title: "dendrite_graph_god_nodes"
sidebar_label: "dendrite_graph_god_nodes"
---

# dendrite_graph_god_nodes

List god nodes (symbols with the most callers and type users) in the project code graph

## Input Schema

```json
{
  "properties": {
    "limit": {
      "description": "Maximum number of god nodes to return (default: 50)",
      "type": "integer"
    },
    "min_in_degree": {
      "description": "Minimum in-degree threshold (default: 21 for FEAT-045 god nodes)",
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
      "min_in_degree": 1,
      "project_path": "'.'"
    },
    "name": "dendrite_graph_god_nodes"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_graph_god_nodes '{"limit":1,"min_in_degree":1,"project_path":"'.'"}'
```
