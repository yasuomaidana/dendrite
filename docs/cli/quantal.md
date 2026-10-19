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
| `--all, -a` | Stage all modified and untracked files |
| `--allow-mocks` | Bypass the zero-mock gate (emergencies only) |
| `--amend` | Amend the previous commit |
| `--dry-run` | Show what would happen without changing anything |
| `--files, -f` | Files to stage (comma-separated) |
| `--local` | Generate the message with the local model (default) (default: true) |
| `--message, -m` | Commit message (skips message generation) |
| `--model` | LLM model name (default: qwen3.8:27b) |
| `--no-local` | Do not use the local model for the message |
| `--no-push` | Do not push after committing |
| `--no-smart` | Do not resolve the ticket automatically |
| `--smart` | Resolve the active ticket from Plane when --ticket is not given (default) (default: true) |
| `--ticket, -t` | Ticket identifier (e.g. DEN-123) |

## Examples

```bash
dendrite quantal --ticket DEN-123
dendrite quantal -t DEN-123 --no-smart -m "fix(cli): handle empty args"
dendrite quantal --files cmd/a.go,cmd/b.go --dry-run
```
