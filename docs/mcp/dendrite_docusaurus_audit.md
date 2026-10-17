---
id: dendrite_docusaurus_audit
title: "dendrite_docusaurus_audit"
sidebar_label: "dendrite_docusaurus_audit"
---

# dendrite_docusaurus_audit

Audit documentation completeness, frontmatter compliance, and feature traceability against codebase packages and Plane tickets

## Input Schema

```json
{
  "properties": {
    "docs_dir": {
      "description": "Documentation source directory (default: docs)",
      "type": "string"
    },
    "project_root": {
      "description": "Project root directory (default: .)",
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
      "docs_dir": "docs",
      "project_root": "."
    },
    "name": "dendrite_docusaurus_audit"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_docusaurus_audit '{"docs_dir":"docs","project_root":"."}'
```
