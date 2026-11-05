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
| `--all` | Sync every non-archived synapse (daemon mode) |
| `--dry-run` | Report what would happen without pushing or writing |
| `--force-plane` | Push DEV_STATUS even if unchanged since the last sync |
| `--git-only` | Only push git commits |
| `--interval` | Watch period, e.g. 5m (default: $DENDRITE_SYNC_INTERVAL or 10m) |
| `--no-fetch` | Skip fetching remotes before computing ahead/behind |
| `--no-rag` | Do not refresh each member's code index |
| `--plane` | Plane project for a repository that is in no synapse (default: $PLANE_PROJECT_ID) |
| `--plane-only` | Only reconcile Plane states and push DEV_STATUS |
| `--synapse` | Sync only the members of this synapse |
| `--watch` | Keep running, syncing every interval (combine with --all for a daemon) |

## Examples

```bash
dendrite sync
dendrite sync --all --watch --interval 10m
dendrite sync --synapse dendrite-ecosystem --dry-run
dendrite sync --all --git-only
```
