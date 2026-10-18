---
name: dendrite-workflow
description: Dendrite-first workflow for every model (Claude, Antigravity/AGY, Gemini, Copilot). Use before starting a session, creating a branch, clone or worktree, committing, or delegating work to another model.
---

# Dendrite-First Workflow

> Managed by `dendrite env prepare`; edits here are overwritten. Change `pkg/env/bootstrap/workflow.go` instead.

Every agent session in this repository runs through Dendrite:

1. Start every session with `dendrite session resume`; it also removes merged, clean working copies.
2. Pick work from Plane with `dendrite plane next` (or `dendrite_ticket_smart_select`), never from `DEV_STATUS.md`.
3. Need an isolated checkout? Run `dendrite work new <ticket>`. It creates a git worktree in `dendrite-work/` next to the main checkout. Never hand-make `git clone` or `git worktree add ../<dir>` copies: those mix with human projects and are never cleaned up.
4. Commit with `dendrite commit` (it references the ticket and pushes to origin; never to main/master).
5. Route every LLM call through Dendrite: the LiteLLM gateway (`http://localhost:4000`) for model access and `dendrite agent jobs submit --isolate` to delegate to Antigravity (AGY) or other models. Do not call provider APIs or spawn other agent CLIs directly.
6. Run `dendrite session checkpoint` after each finished ticket and before long waits.
7. When a copy is finished and merged, `dendrite work gc` (or the next resume/checkpoint) deletes it; `dendrite work done <name>` removes one explicitly.

## Working copies

```bash
dendrite work new DEN-123          # ../dendrite-work/<repo>-DEN-123 on branch agent/den-123
dendrite work list                 # status: dirty, unpushed, merged, upstream gone
dendrite work done DEN-123         # remove one copy (refuses unsaved work without --force)
dendrite work gc [--dry-run]       # remove merged + clean copies
dendrite work adopt [--dry-run]    # move legacy sibling worktrees into dendrite-work/
```

A copy is deleted automatically only when its branch is merged into main/master/develop (or its upstream branch was deleted) **and** it has no uncommitted or unpushed work. Copies used by in-flight agent jobs, the current directory, and story-fork variants are never removed.

## Delegating to other models

```bash
dendrite agent jobs submit --ticket DEN-123 --agent agy --scope pkg/foo --prompt-file prompt.md --isolate
```

`--isolate` runs the job in the ticket's `dendrite-work/` copy, so the delegated model never edits the orchestrator's checkout.
