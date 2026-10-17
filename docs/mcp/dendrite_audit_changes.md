---
id: dendrite_audit_changes
title: "dendrite_audit_changes"
sidebar_label: "dendrite_audit_changes"
---

# dendrite_audit_changes

Topological vendor audit and rebase engine for local/* stacked branches. Validates diffs, runs test suites, non-fast-forward merges into develop, and rebases downstream branches.

## Input Schema

```json
{
  "properties": {
    "branch": {
      "description": "Optional target branch (e.g. 'local/vcs'). If omitted, audits all pending branches topologically.",
      "type": "string"
    },
    "dry_run": {
      "description": "Preview audit validation and rebase sequence without mutating branches.",
      "type": "boolean"
    },
    "max_tokens": {
      "description": "Max token context ceiling for audit payload.",
      "type": "integer"
    },
    "model": {
      "description": "Inference model for audit review (default: qwen3.8:27b).",
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
      "branch": "sample",
      "dry_run": false,
      "max_tokens": 1,
      "model": "qwen3.8:27b)."
    },
    "name": "dendrite_audit_changes"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_audit_changes '{"branch":"sample","dry_run":false,"max_tokens":1,"model":"qwen3.8:27b)."}'
```
