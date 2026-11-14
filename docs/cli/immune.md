---
id: immune
title: "dendrite immune"
sidebar_label: "immune"
---

# dendrite immune

Scan agent skills with SkillSpector before they are installed

## Category

Glial Regulation & Pruning

## Aliases

None

## Subcommands

| Subcommand | Description |
| :--- | :--- |
| `scan` | Scan a skill and print approve / hold / quarantine |

## Flags

None

## Subcommand Reference

### `dendrite immune scan`

Scan a skill and print approve / hold / quarantine

```bash
dendrite immune scan <skill-path> [flags]
```

| Flag | Description |
| :--- | :--- |
| `--approve-max` | highest score that is approved (default: immune.approve_max of the directives) (default: 20) |
| `--hold-max` | highest score that is held; above it is quarantined (default: immune.hold_max of the directives) (default: 50) |
| `--llm` | also run the scanner's model pass |

## Examples

```bash
dendrite immune scan .claude/skills/my-skill
dendrite immune scan ./skills --json
```
