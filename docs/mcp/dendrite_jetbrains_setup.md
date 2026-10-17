---
id: dendrite_jetbrains_setup
title: "dendrite_jetbrains_setup"
sidebar_label: "dendrite_jetbrains_setup"
---

# dendrite_jetbrains_setup

Configure JetBrains IDE toolset with Dendrite MCP servers (.ai/mcp/mcp.json), agent run configurations, and external tools

## Input Schema

```json
{
  "properties": {
    "global_only": {
      "description": "Configure only user-level global files (~/.ai/mcp/mcp.json)",
      "type": "boolean"
    },
    "project_dir": {
      "description": "Project root directory (default: current working directory)",
      "type": "string"
    },
    "project_only": {
      "description": "Configure only project-level files (.ai/mcp/mcp.json and .idea)",
      "type": "boolean"
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
      "global_only": false,
      "project_dir": "current working directory",
      "project_only": false
    },
    "name": "dendrite_jetbrains_setup"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_jetbrains_setup '{"global_only":false,"project_dir":"current working directory","project_only":false}'
```
