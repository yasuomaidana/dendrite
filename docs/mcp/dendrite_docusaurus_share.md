---
id: dendrite_docusaurus_share
title: "dendrite_docusaurus_share"
sidebar_label: "dendrite_docusaurus_share"
---

# dendrite_docusaurus_share

Package Docusaurus documentation into a portable distribution bundle (.tar.gz) with cryptographic SHA-256 sharing-manifest.json

## Input Schema

```json
{
  "properties": {
    "bundle_path": {
      "description": "Destination archive path (default: \u003csite_dir\u003e/dendrite-docs.tar.gz)",
      "type": "string"
    },
    "site_dir": {
      "description": "Generated Docusaurus site directory (default: website)",
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
      "bundle_path": "\u003csite_dir\u003e/dendrite-docs.tar.gz",
      "site_dir": "website"
    },
    "name": "dendrite_docusaurus_share"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_docusaurus_share '{"bundle_path":"\u003csite_dir\u003e/dendrite-docs.tar.gz","site_dir":"website"}'
```
