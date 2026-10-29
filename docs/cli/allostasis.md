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
| `coverage` | Per-package coverage: --ratchet fails on a drop, --update raises the baseline, --floor or --min fails below a minimum |
| `gate` | Run the project's quality gates: production lines per file, repeated code, Rust test structure and coverage |
| `hitl-sync` | Poll the GitHub issue of every Plane To Verify ticket; closed as completed moves it to Done |

## Flags

None

## Subcommand Reference

### `dendrite allostasis coverage`

Per-package coverage: --ratchet fails on a drop, --update raises the baseline, --floor or --min fails below a minimum

```bash
dendrite allostasis coverage [package...] [flags]
```

| Flag | Description |
| :--- | :--- |
| `--baseline` | Baseline JSON file (default: .github/coverage_baseline.json) |
| `--floor` | Use the minimum and the exemptions of .github/dendrite/directives.toml |
| `--min` | Fail when a package is below this coverage percent (test helpers are exempt) |
| `--ratchet` | Fail when any package drops below its baseline |
| `--update` | Raise the baseline to the measured coverage and add new packages |

### `dendrite allostasis gate`

Run the project's quality gates: production lines per file, repeated code, Rust test structure and coverage

```bash
dendrite allostasis gate [flags]
```

| Flag | Description |
| :--- | :--- |
| `--format` | text, json or markdown (default: text) |
| `--only` | Run only these checks: [loc clones rstest coverage] |
| `--path` | Project directory (default: .) |
| `--since` | Judge only what changed since this ref (for example origin/develop) |
| `--skip` | Do not run these checks |
| `--ticket` | Judge only the work done for this ticket |

```bash
dendrite allostasis gate
dendrite allostasis gate --since origin/develop --format markdown
dendrite allostasis gate --only loc,clones
```

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
dendrite allostasis gate
dendrite allostasis coverage --ratchet
dendrite allostasis coverage --update pkg/vcs
dendrite allostasis audit pkg/vcs
```
