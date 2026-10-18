# Agent Quota Recovery & Multi-Agent Coordination

How Dendrite keeps work moving when any agent — including the orchestrating session — runs out of quota.
Tickets: DEN-988 (ledger + classifier), DEN-989 (dispatcher CLI), DEN-990 (fallback chain), DEN-991 (verify-only watcher), DEN-992 (session checkpoint/resume), DEN-993 (spike, this page's findings).

## Goals

- Any agent can hit its quota and the work **continues or pauses without loss**.
- Work **resumes without re-exploration** (same conversation, or a handover capsule for a different agent).
- A new session — the same agent after its reset, a different agent, or a human — picks up from a compact checkpoint.

Policy:

- On quota exhaustion: **other cloud** (GitHub Copilot via LiteLLM) → **local** OxiLLaMa/Ollama (complexity ≤ 4) → **park** until the provider resets.
- When the orchestrator is unavailable, the background watcher **verifies only**. It never commits, moves Plane states or touches GitHub.

## Building blocks

| Component | Command / package | Role |
|---|---|---|
| Job ledger | `pkg/agentjobs`, `.dendrite/jobs/<id>/` | `job.json` state, `events.jsonl` write-ahead log, prompt, handover capsule, agent log, `done` marker |
| Outcome classifier | `pkg/agentjobs/classify.go` | Maps exit code + log tail + file-scope changes to `QUOTA`, `TRANSIENT`, `PERMISSION`, `EARLY_EXIT`, `DONE` |
| Dispatcher | `dendrite agent jobs submit/list/show/tail/wait/retry/cancel` | Starts detached jobs with the standard headless preamble and a file-scope guard |
| Fallback chain | `pkg/agentjobs/fallback.go` + `pkg/scheduler/governor` | Escalates on `QUOTA`, marks exhausted providers until reset, parks with `resume_after` |
| Watcher | `dendrite agent jobs daemon [--install]` | Verifies finished jobs (gofmt, build, vet, race tests, coverage), re-dispatches failures once, wakes parked jobs |
| Handoff | `dendrite session checkpoint` / `dendrite session resume` | < 500-token state checkpoint; `resume` is the first command of every new session |

## Spike findings (DEN-993, 2026-10-17)

**AGY headless JSON output** (`agy -p … --output-format json`) returns:

```json
{"conversation_id": "159b3f22-…", "status": "SUCCESS", "response": "OK",
 "duration_seconds": 2.28, "num_turns": 1,
 "usage": {"input_tokens": 11995, "output_tokens": 1, "thinking_tokens": 0, "cache_read_tokens": 0, "total_tokens": 11996}}
```

- `conversation_id` must be stored in `job.json`.
- `agy --conversation <id> -p "continue …"` resumes with full memory (verified: the resumed turn recalled its previous answer).
- The resumed call re-sends history (24,209 input tokens versus 11,995 for a fresh call). It is still far cheaper than re-exploring a codebase, so `TRANSIENT` failures resume the same conversation.
- Even a trivial call carries about 12k tokens of fixed overhead. Batch small tasks per job instead of firing many tiny jobs.
- `usage` is recorded per attempt in `events.jsonl` and feeds per-ticket attribution (`dendrite tokens`).
- In JSON mode the `conversation_id` is printed only when the turn ends, so a killed or stalled job loses it. Bound every headless run with `--print-timeout`, and run long jobs one at a time per workspace.

**AGY failure signatures observed in real runs:**

| Signature | Example | Class |
|---|---|---|
| exit 3 + `AGY_ERROR {"short_error":"…request failed: Post …: EOF","status":"UNKNOWN"…}` + "network issue connecting to the server" | DEN-982 first run | `TRANSIENT` |
| exit 3 + `AGY_ERROR {"short_error":"The stream was interrupted. Please continue the task you were working on.","retryable":false}` — flagged non-retryable but continuable | DEN-910 fix run | `TRANSIENT` |
| exit 0 + `jetski: no output produced — a tool required the "command" permission … auto-denied` | audit batches, DEN-900 first run | `PERMISSION` |
| exit 0 + "launched a … subagent" and no file changes | DEN-910 first run | `EARLY_EXIT` |
| exit 0 + changes inside the file scope | most jobs | `DONE` |
| every tier answers, but a one-word reply takes ~15 min (vs 2 s) and `--print-timeout` is ignored, after a day of heavy use | 2026-10-18 00:30 probes | `QUOTA` (throttled): detected by latency, not by an error string; the dispatcher enforces its own wall-clock deadline (kill the process group) and falls back to Copilot via LiteLLM |
| process alive, no file or log progress for > 30 min, near-zero CPU (AGY language server shut down underneath it) | DEN-910 rewrite + DEN-989 fix, launched in the same second | `STALLED`: the watcher kills the process group and resumes the conversation; parallel AGY jobs are started a few seconds apart |

**Quota exhaustion signatures.** None were observed yet, so these come from documented provider conventions and are kept in one table in `classify.go`:

- **Gemini / AGY:** `RESOURCE_EXHAUSTED` status or error code, HTTP `429`, "quota", "exhausted your capacity".
- **LiteLLM (any upstream, incl. Copilot):** `litellm.RateLimitError` / HTTP 429. Budget and virtual-key caps surface as `BudgetExceededError` (HTTP 400). Both are treated as `QUOTA`.
- **Local engines (OxiLLaMa / Ollama):** no quota. Connection refused means `TRANSIENT`, handled by auto-start (DEN-923).

**No local quota readout.** The AGY CLI keeps an internal `quota_manager` but exposes no remaining-quota value on disk or via a subcommand. Detection is therefore reactive (the classifier), plus proactive spend tracking from `usage` against the weekly policy in `pkg/scheduler/token`.

## Flow

```mermaid
stateDiagram-v2
    [*] --> Running: dendrite agent jobs submit
    Running --> Done: exit 0, changes in scope
    Running --> Transient: network / 5xx
    Running --> Quota: 429 / RESOURCE_EXHAUSTED / RateLimitError
    Running --> EarlyExit: no changes / subagent
    Running --> Permission: auto-denied command
    Transient --> Running: backoff, resume same conversation (max 3)
    EarlyExit --> Running: retry once, no-subagent preamble
    Permission --> Running: retry once, file-tools preamble
    Quota --> Running: next tier (Copilot, then local if C<=4) + capsule
    Quota --> Parked: no tier left
    Parked --> Running: resume_after passed and provider has quota
    Done --> Verified: watcher: gofmt, build, vet, race tests, coverage
    Done --> Running: verification failed, re-dispatch once with failure capsule
    Verified --> Committed: orchestrator only (dendrite commit, To Verify, HITL issue)
```

## End-to-end verification (2026-10-18)

A real job ran in a throwaway Go module:

1. `dendrite agent jobs submit --agent agy --model gemini-3.8-flash-low --scope calc/` → one attempt, classified `DONE`, status `succeeded`.
2. `dendrite agent jobs daemon --once` → `verified` with gofmt, build, vet and tests all `ok`, and per-package coverage recorded. Nothing was committed.

The first run surfaced two bugs, both fixed:

- **State dir counted as agent edits.** The ledger lives inside the repository by default (`<repo>/.dendrite`), so the job's own bookkeeping counted as out-of-scope edits. The scope check now ignores `.dendrite/` and the ledger root.
- **Success classified as `TRANSIENT`.** A clean exit was misread because the agent's summary contained a path with `-501`, and a bare `50x` number matched the 5xx pattern. Bare numbers no longer match. A clean exit (code 0) without an `AGY_ERROR` envelope is never classified `QUOTA` or `TRANSIENT`, because quota or 5xx words in an agent's own summary are content.
