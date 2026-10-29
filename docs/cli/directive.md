---
id: directive
title: "dendrite directive"
sidebar_label: "directive"
---

# dendrite directive

Change what CI, the quality gate and the agents enforce: lines per file, coverage, duplication, who sets Done

## Category

Glial Regulation & Pruning

## Aliases

- `directives`

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `apply` | Regenerate the files derived from the directives |
| `list` | Show the directives in force |
| `set` | Set directives directly, without reading a rule |

## Flags

None

## Subcommand Reference

### `dendrite directive set`

Set directives directly, without reading a rule

```bash
dendrite directive set <key=value>...
```

```bash
dendrite directive set coverage.min_percent=95 loc.warn_at=400
```

## Examples

```bash
dendrite directive "enforce max 500 lines of code ignoring tests"
dendrite directive "enforce 95% code coverage" -y
dendrite directive set coverage.min_percent=95 loc.warn_at=400
dendrite directive list
```
