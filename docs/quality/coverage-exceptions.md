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
| `pkg/planesvc/planetest`, `pkg/rag/ragtest`, `pkg/coreclient/coretest` | Test-support packages that start the real Rust sidecars (or, for `coretest`, a scripted stand-in of `dendrite-core`). Other packages exercise them through `TestMain`; the sidecar lookup cannot run hermetically. | None; they are test infrastructure. |
| `pkg/stacks` (83.3%) | Not touched by the release beyond a test helper; the gap was already there. | Part of the DEN-414 debt. |
| `cmd/dendrite/chronos` (89.2%), `cmd/dendrite/mesh` (85.6%), `pkg/agent/triage` (96.8%), `pkg/devtool` (96.0%), `pkg/env` (90.9%), `pkg/gateway` (88.8%), `pkg/query/transpile` (87.3%), `pkg/rag` (98.9%) | The baseline holds what the CI runner measures, up to 1.4 points below a developer machine: tests that need git, a non-root user, local services or the sidecars cover less there. | Part of the DEN-414 debt: run these packages under the CI conditions, cover the gaps, then raise the baseline. |

The `pkg/installer` and `pkg/memfit` gaps that only showed on Linux (DEN-915) are closed: both are at 100.0% on macOS and Linux.

## Duplication

`dendrite allostasis gate --since origin/main` still reports repeated blocks that this release did not add: `errors.New` var blocks in type files, older table-style tests, and register boilerplate in `pkg/mcpserver`. The blocks this release introduced were removed. The rest stay under DEN-414.
