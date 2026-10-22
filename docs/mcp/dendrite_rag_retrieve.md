---
id: dendrite_rag_retrieve
title: "dendrite_rag_retrieve"
sidebar_label: "dendrite_rag_retrieve"
---

# dendrite_rag_retrieve

Retrieve code and documentation chunks from the project RAG index. With detail "map" it returns outlines (doc comment or heading and declaration) of a token-budgeted context pack, a few dozen tokens each: read the map, then fetch the bodies you need with dendrite_rag_show

## Input Schema

```json
{
  "properties": {
    "corpus": {
      "description": "Corpora searched: all (default: code and documentation in one ranking), code, or docs",
      "enum": [
        "all",
        "code",
        "docs"
      ],
      "type": "string"
    },
    "detail": {
      "description": "Return a context pack instead of top hits: auto (text of the best spans, outlines of their graph neighbours), full, or map (outlines only)",
      "enum": [
        "auto",
        "full",
        "map"
      ],
      "type": "string"
    },
    "min_score": {
      "description": "Minimum fused relevance score (default: none)",
      "type": "number"
    },
    "project_path": {
      "description": "Target project directory to index and search (default: '.')",
      "type": "string"
    },
    "query": {
      "description": "Concept, symbol, or question to search within project RAG index",
      "type": "string"
    },
    "token_budget": {
      "description": "Token budget of the context pack when detail is set (default: 2500)",
      "type": "integer"
    },
    "top_k": {
      "description": "Number of context chunks to retrieve (default: 4)",
      "type": "integer"
    }
  },
  "required": [
    "query"
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
      "corpus": "code and documentation in one ranking), code, or docs",
      "detail": "sample",
      "min_score": 1,
      "project_path": "'.'",
      "query": "sample",
      "token_budget": 1,
      "top_k": 1
    },
    "name": "dendrite_rag_retrieve"
  }
}
```

### CLI

```bash
dendrite mcp call dendrite_rag_retrieve '{"corpus":"code and documentation in one ranking), code, or docs","detail":"sample","min_score":1,"project_path":"'.'","query":"sample","token_budget":1,"top_k":1}'
```
