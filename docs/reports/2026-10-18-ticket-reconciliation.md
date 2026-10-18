# Ticket Reconciliation (2026-10-18)

This is a follow-up to the [2026-10-17 Done / To Verify audit](./2026-10-17-done-ticket-audit.md) and the [sub-project relocation](../architecture/subprojects.md#ticket-relocation-2026-10-18). It checks every live Plane ticket against three things: the code, the relocation manifest, and the ticket schema. It then fixes what was out of line.

## Summary

- **Relocation integrity:**
  - 1 missed `move` closed (DEND-202 → embedag 9.4.1).
  - 23 `split` originals marked with the `relocated-split` label.
  - 4 umbrellas linked to their first child; 2 stale dependencies removed.
  - 39 copy titles corrected.
  - 20 new tickets classified as `stay`.
- **State corrections:** 9 tickets moved to To Verify and 5 to In Progress. 14 tickets keep their state but got a comment explaining the gap. Every move cites test or coverage output measured on 2026-10-18.
- **Descriptions:** 16 tickets got their missing Context or Verification sections (existing text kept). 4 near-empty tickets were rewritten to the full schema.
- **DEV_STATUS.md** was regenerated after a fix to `dendrite plane replicate`. It covers 258 tickets: 215 live in DEN plus 43 in the sub-projects.

## Relocation fixes

| Gap | Fix |
|---|---|
| DEND-202 (DEN #107) was a `move` to embedag but still open in DEN as To Do | Backlink to embedag `DEV_STATUS.md` 9.4.1, `relocated` label, `Relocated` state, archived |
| DEN-051, DEN-068, DEN-074 (`split` to embedag) had no backlink | Backlink to embedag 9.2.1 / 9.3.1 / 9.2.2 |
| None of the 23 `split` originals were marked | `relocated-split` label, plus a "Relocated part" section naming the child; state unchanged |
| DEN-971 to 974 were not linked to DMCP-1 / DCORE-1 / DGIT-1 / DRAG-1 | `relates_to` added |
| DEN-973 and DEN-974 were `blocked_by` archived originals (#170, #189) | Removed; the copies DGIT-3 and DRAG-2 already carry the dependency |
| 15 more live DEN tickets were `blocked_by` archived originals (e.g. DEN-014 → #28, DEN-934 → #170) | Repointed to the copies (DMCP-21, DGIT-3, …); 3 copy links added (DEN-064 → DMCP-17, DEN-092 → DMCP-14, DEN-934 → DGIT-3); 0 left |
| Copy titles said "(was DEN-175)", which is a sequence number, not a ticket ID | Changed to "(was DEN #175)", or the suffix dropped where it repeated the ticket's own number; split-copy description backlinks fixed the same way |
| DEN-997 to 1016 (seq 217–236) were missing from the manifest | `stay` rows added; MCP/JetBrains tickets DEN-1004 to 1008, 1012 and 1016 got `relates_to` DMCP-1 |
| Sub-project Ticket Maps listed only DEN IDs; DMCP-1 and DEN-973 were shown wrongly | Plane-key cross-reference tables added to all four `RESPONSIBILITIES.md`; DMCP-1 corrected to Backlog; DEN-973 corrected to umbrella |

## State corrections

| Ticket | Was | Now | Evidence |
|---|---|---|---|
| DEN-997 | Backlog | To Verify | commit 0514066, merged in 89fb518 |
| DEN-993 | Backlog | To Verify | `docs/architecture/agent-quota-recovery.md` covers AC-1 and AC-2 (98934c5) |
| DEN-912 | In Progress | To Verify | `pkg/assert` 100.0%; `toolchain.go`; `COMPLEXITY_EXCEEDED` verdict (7a14700) |
| DEN-988 | In Progress | To Verify | `ledger.go`, `classify.go` 100% (98934c5) |
| DEN-989 | In Progress | To Verify | `dispatch.go` 97.1%, tests green |
| DEN-990 | In Progress | To Verify | `fallback.go` 99.5%, tests green |
| DEN-992 | In Progress | To Verify | `handoff.go` 99.8%; `cmd/dendrite/session.go` tests green |
| DEN-978 | In Progress | To Verify | commit daa9589; `go test -race ./pkg/agent/...` 6.6s with daemons up |
| DEN-024 | To Do | To Verify | 4 commits (real HostRunner and agent runners); `pkg/agent` 98.6% |
| DEN-902 | Backlog | In Progress | compose wiring done (b57fae6); AC-1 key generator `env secrets ensure` missing |
| DEN-945 | Backlog | In Progress | AC-1 done (100%); AC-2 live 40k fallback test pending |
| DEN-994 | Backlog | In Progress | `pkg/mcpserver` 83.7% (target 100%) |
| DEN-995 | Backlog | In Progress | `pkg/server` 98.6% (target 100%) |
| DEN-996 | Backlog | In Progress | `pkg/devtool` 96.5% (target 100%) |

These keep their state, with a comment explaining the gap:

| Ticket(s) | Comment |
|---|---|
| DEN-991 | `TestWatcher_StallDetection_WithFakeClockAndLiveProcess` fails deterministically; `watcher.go` at 69.5% |
| DEN-920 | `dendrite model list` and bench ingestion are missing |
| DEN-981 | AC-4 (`gateway copilot-login`) is missing |
| DEN-910 | AC-2 is at 68.4% (target 70%) |
| DEN-809, DEN-821 | No implementation yet |
| DEN-998 to 1005 | Stay in To Verify; `feat/synapse-mesh` was merged into `heal/qa-m0` in 89fb518 (not yet in `develop`) |

## Descriptions

- **Full rewrite** to Context / Targets & Entry Points / Acceptance Criteria / Verification / DoD, using real file and line references:
  - DEN-098
  - DEND-301 (#109)
  - DEN-818 (hardcoded Tailscale IPs)
  - DEN-819 (hardcoded LiteLLM URL and virtual key)
- **Context added**, using the DEN-917 audit verdicts and entry points: DEN-051, 054, 055, 058, 059, 071, 074, 075, 078, 079, 082, 083, 090.
- **Verification Commands added:** DEN-051, 054, 055, 066, 074, 075, 078, 079, 082, 086, 090, 094.
- DEN-074 now notes that the code is in `pkg/tracker/dag`, not the `pkg/stories/dag` its title names.

## `plane replicate` fix

`pkg/tracker/plane/replicate.go`, regression test `TestPullStatusFromPlane_RelocatedCopiesUseSubProjectIDsAndStates`. Four defects were fixed:

1. Sub-project issues were looked up in DEN's state table, so every copy showed as Backlog. States now come from each project.
2. Copies kept their `[DEN-xxx]` title and were listed under the DEN ID, which duplicated them. They are now listed as `DCORE-6` and so on, with the original title kept.
3. `DEND-*` tickets that live in DEN were split out into a fake `DEND` project section. They now stay in their DEN milestones.
4. The ID regex removed every ticket ID from a title, so "after DEN-972" became "after ". It also truncated follow-up IDs such as `DEN-701-COV-1`. Now only the leading ID is stripped, and suffixed IDs are kept whole.

## Open items for the owner

- **Duplicate ID DEN-809:** Plane #137 (model migration engine) and #139 (inference concurrency limiter, which is what the DEN-809 commits implement) share the ID. One of them needs a new ID so that exact ticket matching and DEV_STATUS.md stay unambiguous.
- **Uncommitted work:** the agent-jobs, assert, catalog and synapse work was committed on `heal/qa-m0` during this pass (98934c5 to 89fb518). This report, the `plane replicate` fix, DEV_STATUS.md, `subprojects.md` and the sub-repo `RESPONSIBILITIES.md` updates are still uncommitted.
- **Docs gate:** `dendrite doc audit --strict` reports missing CLI reference pages for `synapse`, `session` and `work`. These commands came from the merge and from in-progress work in another session.
