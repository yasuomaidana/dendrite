---
id: dendrite_local_agent_triage
title: "dendrite_local_agent_triage"
sidebar_label: "dendrite_local_agent_triage"
---

# dendrite_local_agent_triage

Intercepts developer prompts, evaluates local compute across multi-provider inference (Ollama, Ollama Validated, Hugging Face, Candle/OxiLLaMa), routes chores to zero-cost local models, and enhances/compacts prompts with project skills for cloud escalation

## Input Schema

```json
{
  "properties": {
    "active_file": {
      "description": "Optional active file path in the IDE editor",
      "type": "string"
    },
    "execute_local": {
      "description": "Execute chore tasks locally if local inference is available",
      "type": "boolean"
    },
    "file_content": {
      "description": "Optional source code content of active file",
      "type": "string"
    },
    "force_tier": {
      "description": "Optional tier override: 'local', 'hybrid', 'cloud'",
      "type": "string"
    },
    "prompt": {
      "description": "Developer prompt or chat message",
      "type": "string"
    },
    "target_engine": {
      "description": "Target inference backend: 'ollama', 'ollama-validated', 'huggingface', 'candle', 'oxillama'",
      "type": "string"
    }
  },
  "required": [
    "prompt"
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
      "active_file": "DEV_STATUS.md",
      "execute_local": false,
      "file_content": "DEV_STATUS.md",
      "force_tier": "sample",
      "prompt": "sample",
      "target_engine": "sample"
    },
    "name": "dendrite_local_agent_triage"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_local_agent_triage '{"active_file":"DEV_STATUS.md","execute_local":false,"file_content":"DEV_STATUS.md","force_tier":"sample","prompt":"sample","target_engine":"sample"}'
```
