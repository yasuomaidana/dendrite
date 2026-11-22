---
name: dendrite-rea
description: Reverse-engineering (REA) work with Dendrite: who owns an REA ticket (Claude refuses, AGY owns), the retro commands and MCP tools, and how to keep evidence small. Use before starting, continuing or handing over an REA ticket, from any agent.
---

# Dendrite REA (reverse engineering)

> Managed by `dendrite soma prepare`; edits here are overwritten. Change `pkg/env/bootstrap/workflow.go` instead.

REA tickets are the ones this project's `[rea]` directive names: the label `rea` or a title starting `[REA]`. `dendrite retro` (aliases `rea`, `retrograde-analysis`) is the command group; the same tools are MCP tools, so Claude, AGY and any other agent can use them directly.

## Who owns an REA ticket

A Claude session that refuses an REA ticket must not leave it half done: run `dendrite retro handoff <ticket> --refused --reason "..."` (MCP `dendrite_rea_handoff`). AGY takes ownership: the refusal is recorded as a decision on the ticket, an AGY job starts in an isolated working copy, the ticket is labelled `owner:agy` and the Claude claim is released. A Claude session cannot claim it again without `--steal`.

The Stop hook does this by itself when the API ends a turn with a refusal, and the session-end hook also when a session ends on a short, tool-free reply that only declines. **AGY sessions** have no such hooks: an AGY session that owns an REA ticket (the `owner:agy` label, or a job started by the handoff) simply works it as above, and may start any other REA ticket itself with `dendrite chronos claim <ticket> --agent agy`.

## Commands and tools

```bash
dendrite retro list                                   # the open-source showcase targets
dendrite retro showcase [target] [--format markdown]  # cleanroom analysis and parity check
dendrite retro handoff DEN-123 --refused --reason "..." # hand a ticket you refuse to AGY
dendrite retro analyze <target> [--format F] [--tier local|paid] [--model M]  # any file or directory (PE, ELF, Mach-O, ASAR/ZIP, JS bundle, source): inspect, write documents, index; prints a bounded evidence bundle and an id
dendrite retro search <id> "CreateFileW" [-k 5]       # read only the best passages of the analysis
dendrite retro pause <id>                             # stop at the next stage; the checkpoint is kept
dendrite retro resume <id> [--tier paid --model M]    # continue, optionally on another tier or model
dendrite retro purge <id> --force                     # delete documents, index and checkpoint
dendrite retro reuse <id> [--synapse S] [-k 10]          # code in this project and its synapse that already does what the analysis found
dendrite retro rank <id>                              # which documents of the analysis to read first
dendrite retro adr <id> [--title T]                    # draft a proposed ADR (a person completes decision and consequences)
dendrite retro analyze <target> --ticket DEN-123       # also leave a note with the summary and the id on the ticket
```

MCP: `dendrite_rea_analyze`, `dendrite_rea_search`, `dendrite_rea_pause`, `dendrite_rea_resume`, `dendrite_rea_purge`, `dendrite_rea_reuse`, `dendrite_rea_rank`, `dendrite_rea_adr` and `dendrite_rea_handoff`. The target is inspected by the dendrite-core sidecar (`bin.inspect`; an older one still reads PE images, and `dendrite update --dev` adds the other formats) and searched through the RAG sidecar. Register the server with AGY by running `dendrite soma prepare`; `dendrite soma diagnose` says whether AGY lists it.

## Working an REA ticket (any agent)

1. `dendrite chronos claim <ticket>` (or `dendrite work new <ticket>`), then `dendrite_query_context` for its criteria and plan.
2. Explore with Dendrite before reading raw output: `dendrite rag search`, `dendrite rag symbols`, `dendrite graph`. Search the evidence you already extracted (`dendrite retro search <id> "..."`) instead of dumping it again.
3. Keep every evidence bundle under 4,000 tokens: one function or one table at a time, with its cross-references and string constants.
4. Run `dendrite retro reuse <id>` before writing code, then write an Architecture Decision Record (`dendrite retro adr <id>` drafts it, `docs/architecture/decisions/ADR-XXX.md`) before any implementation: decompile, understand, then recreate. Recreate cleanroom code with unit test vectors; never copy bytes or proprietary text into the repository.
5. Untrusted ELF and PE binaries run only in the sandboxed container, never on the host.
6. Commit with `dendrite quantal`, finish with `dendrite chronos finish`.
