---
id: dendrite_agent_run
title: "dendrite_agent_run"
sidebar_label: "dendrite_agent_run"
---

# dendrite_agent_run

Launch an autonomous coding agent loop (aider, openhands, opencode, agy, copilot) with LiteLLM token governance

## Input Schema

```json
{
  "properties": {
    "agent_type": {
      "description": "Agent tool: 'aider', 'openhands', 'opencode', 'agy', 'copilot'",
      "type": "string"
    },
    "model": {
      "description": "Target LLM model (default: picked from Gemini 3.8 / Argon by load for agy, the DENDRITE_COPILOT_MODEL default for copilot)",
      "type": "string"
    },
    "sandbox": {
      "description": "Execute inside ephemeral container sandbox",
      "type": "boolean"
    },
    "spend_limit": {
      "description": "Maximum USD spend allowed for this session",
      "type": "number"
    },
    "task": {
      "description": "Task description or user prompt for the agent",
      "type": "string"
    },
    "ticket_id": {
      "description": "Associated ticket ID (e.g. DEN-034)",
      "type": "string"
    }
  },
  "required": [
    "agent_type",
    "task"
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
      "agent_type": "sample",
      "model": "picked from Gemini 3.8 / Argon by load for agy, the DENDRITE_COPILOT_MODEL default for copilot",
      "sandbox": false,
      "spend_limit": 1,
      "task": "sample",
      "ticket_id": "DEN-985"
    },
    "name": "dendrite_agent_run"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_agent_run '{"agent_type":"sample","model":"picked from Gemini 3.8 / Argon by load for agy, the DENDRITE_COPILOT_MODEL default for copilot","sandbox":false,"spend_limit":1,"task":"sample","ticket_id":"DEN-985"}'
```
