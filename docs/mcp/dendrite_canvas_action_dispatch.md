---
id: dendrite_canvas_action_dispatch
title: "dendrite_canvas_action_dispatch"
sidebar_label: "dendrite_canvas_action_dispatch"
---

# dendrite_canvas_action_dispatch

Execute a Canvas interactive writing action (15 modes) or UI creative action handler with full context assembly and tone/diff formatting

## Input Schema

```json
{
  "properties": {
    "action": {
      "description": "Canvas Action ID (e.g. WRITE_EMAIL, WRITE_PLAN, WRITE_REPORT, DESIGN_UI_COMPONENT, SYNC_PENPOT_FRAME, RELATE_TICKET_FEATURE, UPDATE_PROJECT_STATE)",
      "type": "string"
    },
    "current_content": {
      "description": "Existing text or code for refinement, expansion, or diff critique",
      "type": "string"
    },
    "mode": {
      "description": "Writing mode: DRAFT, REFINE, EXPAND, CONDENSE, CHANGE_TONE, CRITIQUE, FORMAT (default: DRAFT)",
      "type": "string"
    },
    "parameters": {
      "description": "Optional parameters (framework, component_name, tone, ticket_id, file_path, target_state, dry_run)",
      "type": "object"
    },
    "prompt": {
      "description": "Directive, prompt, or acceptance criteria guiding content generation",
      "type": "string"
    },
    "title": {
      "description": "Title or subject of the creative artifact",
      "type": "string"
    }
  },
  "required": [
    "action",
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
      "action": "sample",
      "current_content": "sample",
      "mode": "DRAFT",
      "parameters": {},
      "prompt": "sample",
      "title": "sample"
    },
    "name": "dendrite_canvas_action_dispatch"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_canvas_action_dispatch '{"action":"sample","current_content":"sample","mode":"DRAFT","parameters":{},"prompt":"sample","title":"sample"}'
```
