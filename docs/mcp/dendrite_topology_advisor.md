---
id: dendrite_topology_advisor
title: "dendrite_topology_advisor"
sidebar_label: "dendrite_topology_advisor"
---

# dendrite_topology_advisor

Evaluate hardware capabilities of compute nodes and calculate optimal placement of Dendrite microservices

## Input Schema

```json
{
  "properties": {
    "nodes": {
      "description": "Comma-separated node definitions (e.g. 'local=localhost,worker=100.84.12.25')",
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
      "nodes": "sample"
    },
    "name": "dendrite_topology_advisor"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_topology_advisor '{"nodes":"sample"}'
```
