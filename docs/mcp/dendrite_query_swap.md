---
id: dendrite_query_swap
title: "dendrite_query_swap"
sidebar_label: "dendrite_query_swap"
---

# dendrite_query_swap

Transforms legacy graphics/IO subsystems into clean trait boundaries and modern adapters (wgpu/threejs) while verifying kinematic parity

## Input Schema

```json
{
  "properties": {
    "from_subsystem": {
      "description": "Current legacy subsystem",
      "enum": [
        "opengl",
        "directx",
        "vulkan",
        "wgpu",
        "threejs"
      ],
      "type": "string"
    },
    "output_dir": {
      "description": "Optional destination directory to emit trait boundary \u0026 adapter",
      "type": "string"
    },
    "sources_path": {
      "description": "Path to legacy application sources",
      "type": "string"
    },
    "target_language": {
      "description": "Target implementation language",
      "enum": [
        "rust",
        "go",
        "typescript"
      ],
      "type": "string"
    },
    "to_subsystem": {
      "description": "Target modern subsystem",
      "enum": [
        "opengl",
        "directx",
        "vulkan",
        "wgpu",
        "threejs"
      ],
      "type": "string"
    }
  },
  "required": [
    "sources_path",
    "from_subsystem",
    "to_subsystem"
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
      "from_subsystem": "sample",
      "output_dir": "sample",
      "sources_path": "DEV_STATUS.md",
      "target_language": "sample",
      "to_subsystem": "sample"
    },
    "name": "dendrite_query_swap"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_query_swap '{"from_subsystem":"sample","output_dir":"sample","sources_path":"DEV_STATUS.md","target_language":"sample","to_subsystem":"sample"}'
```
