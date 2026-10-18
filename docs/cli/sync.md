---
id: sync
title: "dendrite sync"
sidebar_label: "sync"
---

# dendrite sync

Push commits and refresh Plane for this project and its synapse peers

## Category

Terminal Boutons & Vesicles

## Aliases

- `propagate`

## Subcommands

None

## Flags

| Flag | Description |
| :--- | :--- |
| `--synapse` | Sync only the members of this synapse |
| `--all` | Sync every non-archived synapse (daemon mode) |
| `--git-only` | Only push git commits |
| `--plane-only` | Only reconcile Plane states and push DEV_STATUS |
| `--plane` | Plane project for a repository that is in no synapse |
| `--force-plane` | Push DEV_STATUS even if unchanged since the last sync |
| `--no-fetch` | Skip fetching remotes before computing ahead/behind |
| `--dry-run` | Report what would happen without pushing or writing |
| `--json` | Emit the report as JSON |
| `--watch` | Keep running and sync every interval (combine with --all for a daemon) |
| `--interval` | Watch period such as 5m (default: DENDRITE_SYNC_INTERVAL or 10m) |

## Examples

```bash
dendrite sync
dendrite sync --all --watch --interval 10m
dendrite sync --synapse dendrite-mesh --dry-run
dendrite sync --all --git-only
```
