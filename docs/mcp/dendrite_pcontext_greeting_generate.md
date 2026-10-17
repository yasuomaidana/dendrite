---
id: dendrite_pcontext_greeting_generate
title: "dendrite_pcontext_greeting_generate"
sidebar_label: "dendrite_pcontext_greeting_generate"
---

# dendrite_pcontext_greeting_generate

Generates a dynamic 10-style personalized headline greeting with strict cliché suppression, sliding history tracking, and journey context resolution

## Input Schema

```json
{
  "properties": {
    "curiosity_angle": {
      "description": "Novel or unexplored angle for topic",
      "type": "string"
    },
    "days_since_last_active": {
      "description": "Days since the user last interacted",
      "type": "integer"
    },
    "days_until_event": {
      "description": "Days remaining until event",
      "type": "integer"
    },
    "decision": {
      "description": "Decision point user is considering",
      "type": "string"
    },
    "event": {
      "description": "Upcoming event or deadline",
      "type": "string"
    },
    "high_stress": {
      "description": "Whether user context indicates high stress",
      "type": "boolean"
    },
    "low_pressure_hobby": {
      "description": "Whether topic is a low-stakes creative hobby",
      "type": "boolean"
    },
    "milestone": {
      "description": "Milestone title achieved or in-flight",
      "type": "string"
    },
    "milestone_achieved": {
      "description": "Whether the milestone was recently completed",
      "type": "boolean"
    },
    "options": {
      "description": "Option alternatives user is weighing (e.g. Option A vs B)",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "recent_styles": {
      "description": "Recently generated style IDs to suppress (e.g. ['G-01', 'G-02'])",
      "items": {
        "type": "string"
      },
      "type": "array"
    },
    "specific_challenge": {
      "description": "Complex or stressful blocker",
      "type": "string"
    },
    "topic": {
      "description": "User's current topic or project",
      "type": "string"
    },
    "turn_count": {
      "description": "Number of turns in current conversation",
      "type": "integer"
    },
    "unfinished_step": {
      "description": "Pending task left from prior turn",
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
      "curiosity_angle": "sample",
      "days_since_last_active": 1,
      "days_until_event": 1,
      "decision": "sample",
      "event": "sample",
      "high_stress": false,
      "low_pressure_hobby": false,
      "milestone": "sample",
      "milestone_achieved": false,
      "options": [
        "item1"
      ],
      "recent_styles": [
        "item1"
      ],
      "specific_challenge": "sample",
      "topic": "sample",
      "turn_count": 1,
      "unfinished_step": "sample"
    },
    "name": "dendrite_pcontext_greeting_generate"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_pcontext_greeting_generate '{"curiosity_angle":"sample","days_since_last_active":1,"days_until_event":1,"decision":"sample","event":"sample","high_stress":false,"low_pressure_hobby":false,"milestone":"sample","milestone_achieved":false,"options":["item1"],"recent_styles":["item1"],"specific_challenge":"sample","topic":"sample","turn_count":1,"unfinished_step":"sample"}'
```
