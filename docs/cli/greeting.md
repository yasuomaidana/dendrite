---
id: greeting
title: "dendrite greeting"
sidebar_label: "greeting"
---

# dendrite greeting

Generate dynamic 10-style greeting with cliché suppression

## Category

Synaptic Cleft & Gating

## Aliases

- `greet`
- `pcontext-greeting`

## Subcommands

None

## Flags

| Flag | Description |
| :--- | :--- |
| `--challenge` | A hard or stressful challenge |
| `--curiosity` | A curiosity angle or idea |
| `--days` | Days until the event |
| `--days-inactive` | Days since the last interaction |
| `--decision` | Decision being weighed |
| `--event` | Upcoming event or deadline |
| `--hobby` | Low-pressure hobby context |
| `--milestone` | Milestone reached or in flight |
| `--milestone-done` | The milestone was just completed |
| `--options` | Options being weighed (comma-separated) |
| `--stress` | High cognitive load or stress |
| `--suppress` | Style IDs to avoid (e.g. G-01,G-02) |
| `--topic` | Current topic or project |
| `--turns` | Turns in the current conversation (default: 1) |
| `--unfinished` | Unfinished task from the last turn |

## Examples

```bash
dendrite greeting --topic DEN-985
dendrite greeting --event launch --days 3 --stress
dendrite greeting --suppress G-01,G-02 --json
```
