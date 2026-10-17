---
id: dendrite_pool_split
title: "dendrite_pool_split"
sidebar_label: "dendrite_pool_split"
---

# dendrite_pool_split

Extract an AST slice and its tests into a new standalone pool member project

## Input Schema

```json
{
  "properties": {
    "module_subpath": {
      "description": "Relative path to module being extracted (e.g. internal/matcher)",
      "type": "string"
    },
    "new_member_name": {
      "description": "Identifier for the new project member",
      "type": "string"
    },
    "register_in_pool": {
      "description": "Register newly extracted project in .dendrite/pool.json",
      "type": "boolean"
    },
    "source_project_dir": {
      "description": "Path to source project directory",
      "type": "string"
    },
    "target_directory": {
      "description": "Target destination directory on filesystem",
      "type": "string"
    },
    "wire_local_mode": {
      "description": "Wire parent project to depend on new member in local mode",
      "type": "boolean"
    }
  },
  "required": [
    "source_project_dir",
    "module_subpath",
    "new_member_name",
    "target_directory"
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
      "module_subpath": "DEV_STATUS.md",
      "new_member_name": "sample",
      "register_in_pool": false,
      "source_project_dir": "sample",
      "target_directory": "sample",
      "wire_local_mode": false
    },
    "name": "dendrite_pool_split"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_pool_split '{"module_subpath":"DEV_STATUS.md","new_member_name":"sample","register_in_pool":false,"source_project_dir":"sample","target_directory":"sample","wire_local_mode":false}'
```
