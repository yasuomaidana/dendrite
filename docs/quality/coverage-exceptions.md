---
id: coverage-exceptions
title: "Coverage exceptions"
sidebar_label: "Coverage exceptions"
---

# Coverage exceptions

The directive is `min_percent = 100`. Every package touched by release 0.1.0 (DEN-444) is at 100.0%, except the cases below. Each one is written down so it is a decision, not drift.

| Package | Why it is below 100% | Plan |
| :--- | :--- | :--- |
| `cmd/dendrite` | It is the whole CLI (6.4% at baseline). Only the functions changed in this release are held to 100%. | A follow-up ticket covers the package. |
| `pkg/planesvc/planetest`, `pkg/rag/ragtest` | Test-support packages that start the real Rust sidecars. Other packages exercise them through `TestMain`; the sidecar lookup cannot run hermetically. | None; they are test infrastructure. |
| `pkg/stacks` (83.3%) | Not touched by the release beyond a test helper; the gap was already there. | Part of the DEN-414 debt. |

## Duplication

`dendrite allostasis gate --since origin/main` still reports repeated blocks that this release did not add: `errors.New` var blocks in type files, older table-style tests, and register boilerplate in `pkg/mcpserver`. The blocks this release introduced were removed. The rest stay under DEN-414.
