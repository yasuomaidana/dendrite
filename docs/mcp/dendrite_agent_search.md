---
id: dendrite_agent_search
title: "dendrite_agent_search"
sidebar_label: "dendrite_agent_search"
---

# dendrite_agent_search

Searches and routes tasks to optimal autonomous agents (Aider, OpenHands, OpenCode, Google AGY) using hybrid vector (TEI) and lexical (BM25) search with Reciprocal Rank Fusion, Graphify AST Leiden cluster coupling, and historical performance scoring

## Input Schema

```json
{
  "properties": {
    "max_cost": {
      "description": "Optional maximum cost ceiling in USD",
      "type": "number"
    },
    "max_results": {
      "description": "Maximum number of candidate agents to return (default: 5)",
      "type": "integer"
    },
    "query": {
      "description": "Task description or capability search query",
      "type": "string"
    },
    "require_local": {
      "description": "Constrain selection to zero-cost local inference engines (Ollama Metal, OxiLLaMa)",
      "type": "boolean"
    },
    "route": {
      "description": "If true, auto-selects top agent and returns execution routing decision",
      "type": "boolean"
    },
    "target_package": {
      "description": "Optional target Go/Rust package (e.g. 'pkg/ast', 'pkg/gateway')",
      "type": "string"
    },
    "target_symbols": {
      "description": "Optional list of target AST symbols",
      "items": {
        "type": "string"
      },
      "type": "array"
    }
  },
  "required": [
    "query"
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
      "max_cost": 1,
      "max_results": 1,
      "query": "sample",
      "require_local": false,
      "route": false,
      "target_package": "sample",
      "target_symbols": [
        "item1"
      ]
    },
    "name": "dendrite_agent_search"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_agent_search '{"max_cost":1,"max_results":1,"query":"sample","require_local":false,"route":false,"target_package":"sample","target_symbols":["item1"]}'
```
