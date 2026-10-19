---
id: quality
title: "dendrite quality"
sidebar_label: "quality"
---

# dendrite quality

Anti-mock scan, 100% coverage profiler, and HITL verification poller

## Category

Glial Regulation & Pruning

## Aliases

- `homeostatic-audit`
- `allostasis-audit`
- `allostasis`
- `qlty`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `audit` | Run anti-mock scan and verify coverage gates |
| `hitl-sync` | Poll the GitHub issue of every Plane To Verify ticket; closed as completed moves it to Done |

## Flags

None

## Examples

```bash
dendrite quality audit
```
