---
id: allostasis
title: "dendrite allostasis"
sidebar_label: "allostasis"
---

# dendrite allostasis

Anti-mock scan, 100% coverage profiler, and HITL verification poller

## Category

Glial Regulation & Pruning

## Aliases

- `quality`
- `homeostatic-audit`
- `allostasis-audit`
- `qlty`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `audit` | Scan a package for mocks and stubs and report statement coverage (default pkg/vcs) |
| `coverage` | Per-package coverage ratchet: --ratchet fails on a drop, --update raises the baseline |
| `hitl-sync` | Poll the GitHub issue of every Plane To Verify ticket; closed as completed moves it to Done |

## Flags

None

## Subcommand Reference

### `dendrite allostasis coverage`

Per-package coverage ratchet: --ratchet fails on a drop, --update raises the baseline

```bash
dendrite allostasis coverage [package...] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--baseline` | Baseline JSON file (default: .dendrite/coverage_baseline.json) |
| `--ratchet` | Fail when any package drops below its baseline |
| `--update` | Raise the baseline to the measured coverage and add new packages |

### `dendrite allostasis hitl-sync`

Poll the GitHub issue of every Plane To Verify ticket; closed as completed moves it to Done

```bash
dendrite allostasis hitl-sync [flags]
```

Aliases: `sync-hitl`

| Flag | Description |
| :--- | :--- |
| `--dry-run` | Report approvals without moving tickets to Done |
| `--repo, -r` | GitHub repository owner/name (default: the origin remote) |
| `--ticket` | Limit the sync to these ticket IDs (comma-separated) |

```bash
dendrite allostasis hitl-sync --dry-run
dendrite allostasis hitl-sync --ticket DEN-123
```

## Examples

```bash
dendrite allostasis coverage --ratchet
dendrite allostasis coverage --update pkg/vcs
dendrite allostasis audit pkg/vcs
```
