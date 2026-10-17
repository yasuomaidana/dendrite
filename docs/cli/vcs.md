---
id: vcs
title: "dendrite vcs"
sidebar_label: "vcs"
---

# dendrite vcs

Git branch guardrails prohibiting main pushes and ensuring rebase integrity

## Category

Soma & Axon Hillock

## Aliases

- `hillock-guard`
- `myelin-guard`
- `guard`
- `vcs`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `check` | Verify active branch complies with guardrails |
| `guard` | Enforce branch policy and halt if on main/master |
| `status` | Show current VCS status and branch policy |
| `sync-remote` | Fetch and rebase remote fixes with commit reconciliation |
| `evacuate` | Force evacuate working tree to develop branch |

## Flags

None

## Examples

```bash
dendrite vcs check
dendrite vcs sync-remote
```
