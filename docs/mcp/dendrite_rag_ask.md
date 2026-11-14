---
id: dendrite_rag_ask
title: "dendrite_rag_ask"
sidebar_label: "dendrite_rag_ask"
---

# dendrite_rag_ask

Ask architectural, API, or codebase questions with local RAG retrieval and optional paid cloud LLM fallback

## Input Schema

```json
{
  "properties": {
    "auto_approve_paid": {
      "description": "Automatically escalate to paid cloud model if local LLM fails without prompt",
      "type": "boolean"
    },
    "corpus": {
      "description": "Corpora searched: all (default: code and documentation), code (implementation questions), docs (documentation only: design and conceptual questions), or tickets (the Plane tickets of the project: status, blockers, what a ticket is for)",
      "enum": [
        "all",
        "code",
        "docs",
        "tickets"
      ],
      "type": "string"
    },
    "docs_share": {
      "description": "With corpus all: share of the context pack kept for documentation, 0 to 1 (default: the project's [ask] docs_share, else 0.2). Lower leans on code, higher on documentation",
      "maximum": 1,
      "minimum": 0,
      "type": "number"
    },
    "local_model": {
      "description": "Preferred local model",
      "type": "string"
    },
    "paid_model": {
      "description": "Paid cloud model fallback (default: 'gemini-2.5-pro')",
      "type": "string"
    },
    "project_path": {
      "description": "Target project directory to index and search (default: '.')",
      "type": "string"
    },
    "question": {
      "description": "Question to ask about the project",
      "type": "string"
    },
    "top_k": {
      "description": "Number of best chunks to answer from (default: 4); 0 answers from a token-budgeted context pack instead",
      "type": "integer"
    }
  },
  "required": [
    "question"
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
      "auto_approve_paid": false,
      "corpus": "code and documentation), code (implementation questions), docs (documentation only: design and conceptual questions), or tickets (the Plane tickets of the project: status, blockers, what a ticket is for",
      "docs_share": 1,
      "local_model": "sample",
      "paid_model": "'gemini-2.5-pro'",
      "project_path": "'.'",
      "question": "sample",
      "top_k": 1
    },
    "name": "dendrite_rag_ask"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rag_ask '{"auto_approve_paid":false,"corpus":"code and documentation), code (implementation questions), docs (documentation only: design and conceptual questions), or tickets (the Plane tickets of the project: status, blockers, what a ticket is for","docs_share":1,"local_model":"sample","paid_model":"'gemini-2.5-pro'","project_path":"'.'","question":"sample","top_k":1}'
```
