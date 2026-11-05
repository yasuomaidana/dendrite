---
id: dendrite_gateway_update
title: "dendrite_gateway_update"
sidebar_label: "dendrite_gateway_update"
---

# dendrite_gateway_update

Dynamically discover live inference engines (Ollama, OxiLLaMa, etc.), score loaded models by intelligence, and update LiteLLM routing table (default: ~/.dendrite/stacks/litellm_config.yaml)

## Input Schema

```json
{
  "properties": {
    "dry_run": {
      "description": "Simulate update without modifying disk (default: false)",
      "type": "boolean"
    },
    "file": {
      "description": "Target litellm_config.yaml path (default: ~/.dendrite/stacks/litellm_config.yaml)",
      "type": "string"
    },
    "use_container_host": {
      "description": "Use host.docker.internal for container networking (default: true for the stacks directory)",
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
      "dry_run": false,
      "file": "~/.dendrite/stacks/litellm_config.yaml",
      "use_container_host": false
    },
    "name": "dendrite_gateway_update"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_gateway_update '{"dry_run":false,"file":"~/.dendrite/stacks/litellm_config.yaml","use_container_host":false}'
```
