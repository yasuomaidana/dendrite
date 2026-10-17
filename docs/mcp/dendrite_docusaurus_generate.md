---
id: dendrite_docusaurus_generate
title: "dendrite_docusaurus_generate"
sidebar_label: "dendrite_docusaurus_generate"
---

# dendrite_docusaurus_generate

Generate complete Docusaurus v3 Docs-as-Code website, sidebar taxonomy, project sharing hub, and portable manifest from markdown specifications

## Input Schema

```json
{
  "properties": {
    "docs_dir": {
      "description": "Source markdown documentation directory (default: docs)",
      "type": "string"
    },
    "port": {
      "description": "Local preview port (default: 3000)",
      "type": "integer"
    },
    "sharing_tier": {
      "description": "Distribution visibility tier: 'public', 'internal', 'federated' (default: federated)",
      "type": "string"
    },
    "target_dir": {
      "description": "Target Docusaurus site directory (default: website)",
      "type": "string"
    },
    "title": {
      "description": "Site title (default: Dendrite Documentation)",
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
      "port": 1,
      "sharing_tier": "federated",
      "target_dir": "website",
      "title": "Dendrite Documentation"
    },
    "name": "dendrite_docusaurus_generate"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_docusaurus_generate '{"docs_dir":"docs","port":1,"sharing_tier":"federated","target_dir":"website","title":"Dendrite Documentation"}'
```
