---
id: dendrite_test_auto
title: "dendrite_test_auto"
sidebar_label: "dendrite_test_auto"
---

# dendrite_test_auto

Analyzes AST symbol boundaries, detects persistence dependencies (PostgreSQL, Redis, SQLite), scaffolds Testcontainers environments, and executes a self-healing test convergence loop (<= 3 iterations) achieving >= 85% coverage

## Input Schema

```json
{
  "properties": {
    "file": {
      "description": "Path to target source file (e.g. 'pkg/gateway/limiter.go')",
      "type": "string"
    },
    "max_iterations": {
      "description": "Maximum self-healing convergence iterations (default: 3)",
      "type": "integer"
    },
    "with_testcontainers": {
      "description": "Force Testcontainers scaffolding generation even if no persistent dependencies detected",
      "type": "boolean"
    }
  },
  "required": [
    "file"
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
      "file": "DEV_STATUS.md",
      "max_iterations": 1,
      "with_testcontainers": false
    },
    "name": "dendrite_test_auto"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_test_auto '{"file":"DEV_STATUS.md","max_iterations":1,"with_testcontainers":false}'
```
