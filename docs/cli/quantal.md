---
id: quantal
title: "dendrite quantal"
sidebar_label: "quantal"
---

# dendrite quantal

Autonomous Git commit with local model message generation

## Category

Terminal Boutons & Vesicles

## Aliases

- `commit`
- `quantal-commit`
- `cmt`

## Subcommands

None

## Flags

| Flag | Description |
| :--- | :--- |
| `--ticket` | Target ticket ID (e.g. DEN-098) |
| `--message` | Explicit commit message (overrides model synthesis) |
| `--dry-run` | Preview staged changes and message without modifying git history |
| `--no-push` | Opt out of automatic remote push |

## Examples

```bash
dendrite commit --ticket DEN-985
dendrite commit -m "feat(docs): generate cli reference docs"
```
