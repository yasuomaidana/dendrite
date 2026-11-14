# Dendrite Technology Integration Roadmap (2026-11)

> Epic DEN-422; session tickets DEN-423..436 plus the 2026-11-03 MAPI re-sequence tickets DEN-832..837. Source of truth for status is Plane. Execution order: see **Re-sequence for the MAPI MVP** below (it supersedes the stage numbering for ordering; session content is unchanged).


## Context

Three Gemini deep-research documents (`research_Architectural_Role_Classification.md`, `research_Architectural_Evaluation.md`, `research_Architectural_Synthesis_and_Multi-Agent_Orchestration.md`, distilled into the untracked `raw_plan.md`) propose integrating 2026 agent technologies into Dendrite. Every substantive external claim was independently verified (the Gemini share links are sign-in-gated, but the local research files cover them). This plan replaces `raw_plan.md` with a grounded multi-session roadmap: each "session" is one opusplan Claude Code session, tracked as Plane tickets via the Dendrite-first workflow.

**Governing user constraint (2026-11-01): avoid implementing from scratch — it leaves us the maintenance cost. Implement in-project only when (1) the external tool goes against the project/vision, or (2) there is a clear advantage to owning it.** Every item below is tagged ADOPT (use as-is), WRAP (thin glue we own), BUILD (justified ownership), or DEFER.

**User decisions (recorded, must be honored):**
1. **Hermes**: support Hermes Agent as a fourth executor (daemon/cron/gateways disabled, sandboxed, Dendrite orchestrates) — not just pattern-mining.
2. **Memory**: adopt Engram (`Gentleman-Programming/engram`) alongside rag-rs. Engram = agent-memory source of truth; rag-rs = code/docs RAG.
3. **Specs**: committed specs in `docs/specs/<domain>/` (speq-skill layout); per-ticket micro-plans on Plane via `chronos plan push`; `.dendrite/` stays untracked (only derived runtime copies).
4. **MAPI is a use case, not a design target** (2026-11-03): Dendrite derives feature requirements from `~/WebstormProjects/mapi`; mapi-internal work (the package split, the e-shop and DTF-studio apps) belongs to its own Plane project **MAPIFE**, never to DEN tickets.
5. **Bun workspaces, not pnpm** (2026-11-03): the mapi frontend is Bun end-to-end (bun.lock, bunfig preloads, 53 `bun:test` files); Dendrite's pool/meiosis workspace support targets Bun workspaces first.
6. **Kakebo stays paused** in `dendrite store` and the vesicle scheduler while the MAPI MVP runs.

## Adopt-vs-build filter (the heart of this plan)

| Technology | Verdict | Rationale |
|---|---|---|
| **EGM** (google/elephant-goldfish-model, Apache-2.0) | **ADOPT** as plugin in claude 2.1.316 + agy 1.4.12 | First-class plugin for both CLIs; zero build. Config-only fixes: 7-lens merge, `max review loops = 2`, artifact dir → `docs/specs/` |
| **Gentle-AI / Engram** | **ADOPT** binary as-is | Engram already has typed bullets, salience scoring, contradiction resolution, and token-budgeted `mem_context` — most of the "frozen snapshot" is built in. Not installed yet → install, don't rebuild |
| **speq-skill** (Rust CLI + skills) | **ADOPT** as-is | Its micro-spec loading + `[expert]` tagging is exactly what we'd otherwise write. Scanned before install |
| **obra/superpowers** (TDD/debugging skills) | **ADOPT** skills as-is | Install for claude+agy; reference from agent bootstrap templates. No pkg/agent embedding — that was raw_plan over-reach |
| **SkillSpector** (NVIDIA) | **ADOPT** scanner + **WRAP** (`dendrite immune`) | Scanner used as-is (uv tool/Docker, local LLM pass). The wrapper is justified: hooks/gate integration is core Dendrite vision and tickets DEN-388..390 already exist |
| **Hermes Agent** | **ADOPT** as sandboxed executor (user decision) + **WRAP** runner | Upstream v2026.4.30+ settings (`skills.write_approval: true`, background review restricted/off) do the safety work; we only own a bouton runner + config |
| **Harness² runtime** | **REJECT runtime / BUILD-lite patterns** | Runtime is a Python research CLI coupled to OpenCode/Codex/Vertex, sandbox-off — against project vision (criterion 1). tools.toml schema extends existing DEN-953; Level-2 rule extraction is small and high-value. **k=2 propose-probe-compose: DEFER** until vesicle data shows escalations are expensive enough to justify it |
| **HRR trust-decay / dreaming port** | **DEFER** | Engram's native salience + contradiction resolution overlaps most of the value; build only if evidence shows gaps. A thin scheduled job calling Engram's own consolidation = WRAP, allowed |
| **MCP over HTTP + .well-known lazy schemas** | **ADOPT** official MCP go-sdk / spec | The Mar-2026 MCP roadmap standardized `.well-known` lazy loading — adopt the official transport in mcp-hub instead of hand-rolling (session decision point) |
| **ACP/A2A blackboard + leases** | **BUILD** (justified) | Core control-plane vision; design + tickets already exist (DEN-1009..1016, `docs/architecture/synapse-and-agent-mesh.md`). Adopt the A2A `agent-card.json` schema rather than inventing one |
| **saga fork parallel / resolve composer** | **BUILD** (justified) | `saga` is an existing core Dendrite command; Go-interim composer; git-rs semantic compose deferred |
| **Graphify** (`graphifyy` CLI, tree-sitter → knowledge graph) | **Provisional REJECT**; benchmark **DEN-837** decides ADOPT/ADAPT/REJECT | rag-rs claims the same ground natively (`dendrite_graph_*`, FEAT-045) **with** embeddings/BM25 on top, but that is asserted, not measured → head-to-head benchmark (structural accuracy, context efficiency, build cost, edge quality on Go/Rust/TS repos). Regardless of outcome, `graphify install` is never run: its PreToolUse hook fights the dendrite-first hook, its CLAUDE.md rule contradicts the workflow, `graphify-out/` pollutes the RAG index, its git hooks duplicate sidecar re-indexing. If Graphify wins a dimension → **ADAPT**: port the technique into rag-rs (Rust-first). Residual use: manual one-off graphs of non-code media outside indexed repos |
| **Aider** | **EXCLUDE** (installed but banned: repo-map/auto-commit collide with leases) · **OpenCode**: DEFER to a later stage · **OpenClaw**: DEFER (optional notification relay only) · **gemini-4-carbon**: doesn't exist publicly — research tier = gemini-4-argon-high · **gbrain/Hindsight sidecar**: DEFER benchmark |

**Verified corrections kept from the audit**: SkillSpector thresholds 0–20/21–50/51+; prefixes are DEN/DCORE/DRAG/DGIT/DMCP/DPLANE/EMBEDAG (never DN-CT-XXX); core-rs and git-rs are empty stubs — nothing on the critical path depends on them (leases key on rag-rs symbol hashes); `pkg/oxilease` already handles lifecycle (2-min grace) — no new lifecycle manager; known defect `pkg/inference/oxillama_server.go:274` `synthesizeResponse` returns canned text (zero-mock violation, must die early). agy exposes argon/3.8-flash/opus-5.5/sonnet-5.5/gpt-oss-120b. Live: Ollama :11434, LiteLLM :4000, Plane :8090, Penpot :9000; cortex dashboard owns :3456.

## Ground rules (every session)

- Start `dendrite session resume` → `dendrite chronos next`/`create`; one ticket per change; `dendrite work new <ticket>`; `dendrite quantal`; gate (500 LOC / 100% coverage / 0% clones / rstest); `dendrite chronos finish`; end `dendrite session checkpoint`; unfinished plans `chronos plan push`.
- Dogfood: after landing a Dendrite feature, `dendrite update --dev` (pre-approved); next session must use it.
- New tickets carry a routing tag — `[chore-local]` OxiLLaMa/Ollama · `[fast-impl]` gemini-3.8-flash · `[standard-dev]` Sonnet · `[macro-context]` argon · `[research-sec]` argon-high · `[expert]` Opus/Fable — plus `[AC-n]` with exact test commands.
- Opus re-plans each session start (the board moves between sessions); reuse backlog tickets marked **(reuse)**.
- **Execution conventions (user directive 2026-11-01)**: run in auto mode; prefer local models and agy for delegation per the routing tags (`dendrite ask`/`rag` before any raw search, `bouton jobs submit --isolate` for offload); **every ticket's Plane status is kept current at every step** (claim → In Progress → finish/handoff, decisions recorded via `chronos decision`, progress via `chronos comment`).

---

## Re-sequence for the MAPI MVP (2026-11-03, DEN-832)

The MAPI MVP (e-shop + DTF printing studio built on `~/WebstormProjects/mapi`, Plane project **MAPIFE**) is the urgent production driver. MAPI is a **use case**: it tells us which Dendrite features to pull forward, and nothing more. What Dendrite must supply, in order of need: workspace lockstep linking (split the app into Bun workspace packages), polyglot Rust-crate linking (the print-layout nesting module), early HTTP MCP + Penpot design context, and symbol leases so several agents can work the packages concurrently.

Ground truth that shaped the order: mapi is a single Svelte 5 + Vite + Bun app in `frontend/` (no monorepo, no e-shop/DTF app yet, no Rust); Penpot runs healthy on :9000/:9001 but has no access tokens, no exporter and no MCP server (mcp-hub is stdio-only; Penpot upstream = DMCP-2, backlog); `dendrite pool` already exists as an alias of `plexus` (`cmd/dendrite/pool.go`), so the pull-forward extends it; `meiosis` does not exist (DEN-387 backlog).

### Execution order (supersedes stage numbering for ordering; content unchanged)

| # | Ticket | Content | blocked_by | MAPI unlock after it |
|---|--------|---------|------------|----------------------|
| ✅ | DEN-423 (S0) | board reconciliation | — | — |
| ✅ | DEN-424 (S1) | EGM/speq/superpowers, pre-scan | DEN-423 | — |
| 1 | DEN-425 (S2) | `dendrite immune` SkillSpector gate | DEN-424 | agents may safely gain skills for UI work |
| 2 | DEN-426 (S3) | real OxiLLaMa, kill spike mock | DEN-425 | 0-token local checks for high-churn loops |
| 3 | DEN-427 (S4) | Engram + frozen snapshot | DEN-426 | soak clock starts; MAPI co-dev traces count |
| 4 | **DEN-833** | meiosis v1 + pool lockstep linking (Bun workspaces; split out of S13) | DEN-427 | MAPIFE can split `frontend/` into workspace packages |
| 5 | **DEN-834** | pool polyglot: Rust crate ↔ TS packages + `--audit-library-reuse` | DEN-833 | print-layout Rust module scaffold (WASM/NAPI under Bun) |
| 6 | DEN-429 (S6) | tools.toml + Level-2 rules | DEN-834 | governed harness for MAPIFE agents |
| 7 | DEN-430 (S7) | EGM pipeline wiring | DEN-429 | spec'd + drift-gated UI feature work |
| 8 | DEN-431 (S8) | routing, tier tags, vesicle ledger | DEN-430 | cheap-model routing for UI churn |
| 9 | DEN-433 (S10) | MCP over HTTP (go-sdk) + lazy manifest | DEN-431 | HTTP MCP for IDE/agents on mapi |
| 10 | DEN-434 (S11) | ACP blackboard + leases on rag-rs symbol hashes | DEN-433 | concurrent agents on mapi packages without clobbering |
| 11 | **DEN-836** | Penpot context enrichment + 3-way triad glue (with DMCP-2) | DEN-434 (+DEN-835 done) | design-token-enriched prompts for the UI library |
| 12 | DEN-428 (S5) | consolidation + HRR gap decision | DEN-836 | — (soak satisfied by MAPI co-dev) |
| 13 | DEN-432 (S9) | Hermes fourth executor + bench | DEN-428 | optional extra executor |
| 14 | DEN-435 (S12) | saga fork parallel + composer | DEN-432 | parallel probes on mapi features |
| 15 | DEN-436 (S13) | closeout minus meiosis: cadence scheduler + retro | DEN-435 | retro decides deferred items |

Unchained fillers, any slot: DEN-440 (agy telemetry) · DEN-441 (secrets-rotation UX + `cleft update` reconcile of the stale `master_key` in `~/.dendrite/stacks/litellm_config.yaml`) · DEN-443 (S1 leftovers) · **DEN-835** (Penpot service readiness: `enable-access-tokens`, exporter service, PENPOT_ACCESS_TOKEN in secrets — do early, DEN-836 needs it) · **DEN-837** (Graphify benchmark — do early so the verdict is evidence-based).

Rationale: meiosis/pool sits right after S4 (not before) because meiosis scaffolds an immune baseline (needs S2) and co-dev traces only count once Engram exists (S4); governance S6–S8 runs during the soak; S10/S11 advance ahead of Hermes (S9) and parallel saga (S12); S5 closes once ~2 weeks of MAPI-fed traces exist.

### Penpot triad

Readiness audit and the three-edge specification (Penpot → Code via the DMCP-2 upstream over the DEN-433 transport; Code → Penpot via rag-rs symbol hashes; Penpot ↔ Plane frame links next to `[AC-n]` with drift comments) live in [`2026-11-penpot-triad.md`](2026-11-penpot-triad.md); implementation is DEN-835 + DEN-836 (+DMCP-2).

### Graphify

Verdict in the adopt-vs-build table above: provisional REJECT, environment no-install rule unconditional, final ADOPT/ADAPT/REJECT decided by the **DEN-837** benchmark (report: `docs/reports/graphify-benchmark.md` once run).

## Session 0 — Tickets-RAG health + Plane board reconciliation (FIRST PRIORITY, before everything)

The user's directive: the plan must be reflected on the Plane board itself, and the tickets RAG corpus is the tool that makes that reconciliation efficient — so its health comes first.

1. `DEN-new tickets-rag-health` [chore-local] — verify the tickets corpus works end-to-end: `dendrite rag search --corpus tickets`, `dendrite ask --corpus tickets`, MCP `dendrite_rag_retrieve` with corpus `tickets` return current board content (spot-check against `chronos issues`). If stale/broken: reindex/sync (`rag index`, `rag sync`, sidecar health) and fix as the session's first ticket before anything else. AC: a query for a ticket created today is retrievable; answers cite real ticket IDs.
2. `DEN-new board-reconciliation` [expert plan / fast-impl edits] — using the now-healthy tickets RAG, sweep the ~400-ticket board for conflicts with this roadmap's decisions, and update each conflicting ticket (reword, relabel, or set `cancelled`/`backlog` via `chronos ticket set`, with a comment explaining the superseding decision). Known conflict classes to search for:
   - tickets assuming memory lands natively in rag-rs/Dendrite store where **Engram is now adopted** (e.g., DEN-941 retitle to Engram flush; DEN-964/DRAG-2 scope check);
   - tickets assuming `.dendrite/specs` or other committed-state-in-`.dendrite` (specs now live in `docs/specs/`);
   - MCP tickets written stdio-only where S10 adopts the official HTTP transport (e.g., the DEN-1005-era scope);
   - routing/escalation tickets that re-include Aider or predate the agy model list (no gemini-4-carbon);
   - lease/AST tickets that depend on core-rs (stub) instead of rag-rs symbol hashes (DEN-1010 amendment);
   - anything duplicating what's now ADOPTED as-is (snapshot assembly, spec tooling, TDD enforcement in pkg/agent).
3. Create the roadmap epic, create/retitle the Stage A–F tickets with routing tags and ACs, link them (`chronos link`/`relate`), and `chronos plan push` each session's plan onto its tickets. Run `chronos states reconcile` + `sync` so DEV_STATUS and the board agree.

AC for Session 0: tickets RAG answers reflect the reconciled board; no open ticket contradicts a recorded roadmap decision; the epic exists with all stage tickets linked and statuses current.

- **Session 1 first act** (after Session 0): commit this roadmap as `docs/architecture/2026-11-technology-integration-roadmap.md`, delete `raw_plan.md` (research_*.md files move to `docs/reports/research/`).

---

## Stage A — Foundation & trust (Sessions 1–3)

### Session 1 — Adopt the toolchain: EGM, speq-skill, superpowers, agy zero-trust
- `DEN-new adr-artifact-layout` [fast-impl] — ADR: specs → `docs/specs/<domain>/` (speq layout), EGM artifact dir → `docs/specs/`, plans on Plane, `.dendrite/` untracked.
- `DEN-new skillspector-acquire-prescan` [research-sec] — acquire SkillSpector (uv tool or Docker); **scan EGM, superpowers, speq-skill skills BEFORE installing anything**; reports → `docs/reports/skill-scans/`. AC: every skill scored; nothing >50 installed; 21–50 needs a recorded human decision.
- `DEN-new adopt-egm-superpowers-speq` [chore-local/manual] — install EGM plugin (claude + agy), superpowers skills, speq-skill CLI+skills; run `egm-bootstrap`. AC: `/egm:egm-draft` writes under `docs/specs/`; `speq` CLI lists specs; skills visible in both CLIs.
- `DEN-new agy-zero-trust-config` [fast-impl] — telemetry opt-out in `~/.gemini/settings.json`; `.agents/mcp_config.json` `serverUrl` → dendrite-mcp-hub; headless `agy -p` default-deny profile; research pinned to argon-high. AC: committed template in `docs/specs/harness/`; smoke headless JSON run passes.
- **Dogfood hook**: Session 2's immune design is authored with `egm-draft` + one `egm-review` loop.

### Session 2 — Immune: the SkillSpector gate (reuse DEN-388/389/390 — WRAP, not build)
- **DEN-388 (reuse)** `immune scan` [standard-dev] — `cmd/dendrite/immune.go` + `pkg/immune`: wrap the SkillSpector binary (JSON/SARIF parse only, no scanning logic of our own): 0–20 auto-approve / 21–50 hold / 51+ quarantine. AC: `eval(` fixture blocks; clean fixture passes; `go test ./pkg/immune/...`.
- **DEN-389 (reuse)** `immune update` [fast-impl] — pin/refresh scanner version; JSONL scan ledger. AC: idempotent, concurrent-safe.
- **DEN-390 (reuse)** `immune enforce` [standard-dev] — `hook pre-tool`/`pre-edit` deny writes to `.claude/skills/`, `.agents/skills/`, tool_guidance paths without a passing scan at HEAD; LLM pass via LiteLLM :4000 until S3. AC: unscanned skill write blocked with remediation text.
- **Dogfood hook**: every later skill/patch addition passes `immune scan` first.

### Session 3 — Real local inference: kill the `spike serve` mock
- `DEN-new spike-serve-real-oxillama` [standard-dev] — delete `synthesizeResponse` + canned paths; `spike serve` starts/proxies real OxiLLaMa 0.1.4 via existing `pkg/oxilease`. AC: `curl :8080/v1/chat/completions` returns model output; `grep synthesizeResponse` empty; zero-mock gate clean.
- **DEN-920 (reuse, In Progress — merge, don't fork)** capability catalog [fast-impl] — `.dendrite/models.json` entries for agy tiers + local models; `spike bench --save` real throughput.
- `DEN-new immune-llm-pass-local` [chore-local] — immune `--llm` → :8080.
- **Dogfood hook**: later [chore-local] work and EGM goldfish fallbacks run on real OxiLLaMa.

## Stage B — Memory on Engram (Sessions 4–5) — ADOPT + thin glue only

### Session 4 — Engram adoption + frozen session-start snapshot
- `DEN-new engram-adopt` [standard-dev] — `soma prepare` installs/verifies Engram (brew); `motor` health check; declared agent-memory source of truth (rag-rs = code RAG — split-brain avoided by role separation, not sync code). AC: `engram mcp` reachable; `soma diagnose` reports it.
- `DMCP-new engram-upstream` [standard-dev] — register Engram as mcp-hub upstream (already planned in its docs) so every executor reaches memory through one scoped surface. AC: `mem_*` tools listed via `inter mcp tools`.
- `DEN-new session-snapshot-inject` [standard-dev] — `session resume` calls Engram's **own** `mem_context` (token-budgeted: ~800 tok MEMORY + ~500 tok USER) exactly once, injects as frozen prefix; **no snapshot-assembly code of ours — Engram provides it**. AC: second in-session fetch is a no-op; budget respected.
- **DEN-941 (reuse, retitled)** async flush — mid-session writes append to `.dendrite/memory-journal.jsonl` (fsync), `hook session-end` flushes via `mem_save`/`mem_session_summary`; never mutates the injected prefix (KV-cache-safe). AC: kill -9 loses nothing; flush idempotent.
- **Dogfood hook**: Session 5 opens with an injected snapshot of Sessions 1–4.

### Session 5 — Consolidation & curation (evaluate-before-build gate)
- `DEN-new engram-consolidation-job` [fast-impl, WRAP] — `dendrite cognition dream`: a scheduled job invoking **Engram's native** salience/contradiction/merge features (+ summary on local OxiLLaMa); telemetry to sensory/cortex. AC: scheduled run recorded; `--dry-run` reports.
- `DEN-new hrr-gap-evaluation` [research-sec, decision ticket] — run 2 weeks of real usage, then decide with evidence whether Engram's salience suffices or an HRR trust-decay port is warranted (**DEFER default**; record via `chronos decision`).
- `DEN-new skill-curation-minimal` [standard-dev, thin] — minimal JSONL ledger of skill installs/patches/use + a 30d-staleness *report* (proposals only, never auto-archive); patches route through `immune scan`; read-only turns never mutate skills (hook check). Full Hermes-style curator lifecycle **deferred** unless the report shows churn. AC: read-only-turn patch rejected; stale report generated.
- **Dogfood hook**: Session 6's skill/guidance edits flow through ledger + immune.

## Stage C — Harness²-lite and EGM wiring (Sessions 6–7)

### Session 6 — tools.toml + Level-2 rule extraction (k=2 loop deferred)
- **DEN-953 (reuse, extended)** tools.toml [standard-dev] — 8-component schema (`system_prompt, guardrails, memory, skills, tool_guidance, scripts, subagents, plugins`); template `docs/specs/harness/tools.toml`, rendered copy `.dendrite/tools.toml`; `dendrite tool list`. AC: deterministic round-trip + original DEN-953 ACs.
- `DEN-new level2-rule-extraction` [standard-dev] — post-hook contrasts failed vs repaired tool traces → tool_guidance patch *proposals* (JSONL, immune-scanned, applied as a separate approved step). AC: scripted fail→repair fixture yields exactly one proposal naming the failing tool.
- `DEN-new k2-propose-probe-compose` [expert] — **create as DEFERRED backlog ticket only**: build after vesicle data (S8) quantifies escalation cost; Pillar-1 gates as selector when built.
- **Dogfood hook**: Session 7 EGM runs execute under the rendered harness; one real guidance proposal should exist.

### Session 7 — EGM pipeline wiring (config + glue, no reimplementation)
- `DEN-new egm-config` [fast-impl] — 7-lens merge (Codebase-Grounding+Reuse ∪ Observability/SRE), **max 2 review loops**, artifact dir `docs/specs/`, bypass for 1-line fixes. AC: all lenses emitted; third loop refused.
- `DEN-new goldfish-routing` [standard-dev, thin] — bouton wrapper dispatching the EGM plugin's own review stages via `agy -p --output-format json` on 3.8-flash (never Opus/argon for goldfish); verdicts onto the ticket via `chronos comment`; malformed JSON = hard failure. AC: fixture spec → 5 structured verdicts on ticket.
- `DEN-new drift-report-finish-gate` [standard-dev] — tickets referencing `docs/specs/` run `egm-drift-report` inside `chronos finish`; excess drift blocks finish. AC: drifting fixture blocked; spec-less tickets unaffected.
- **Dogfood hook**: every Stage D–F design-bearing ticket is egm-drafted and drift-gated.

## Stage D — Routing + Hermes executor (Sessions 8–9)

### Session 8 — Capability routing, tier tags, token attribution (existing backlog)
- **DEN-921 (reuse)** capability scorer; **DEN-924 (reuse)** escalation ladder — **merge into existing `pkg/agentjobs/fallback.go`**, formal failure classes, capsules; keep QUOTA cooldown + oxilease grace; **DEN-925 (reuse)** all inference through router/lease.
- `DEN-new routing-tier-ticket-tags` [fast-impl] — chronos lint for the six tags; tags map to ladder start tiers (speq-skill's `[expert]` convention adopted, not reinvented). AC: `chronos next` surfaces tags; untagged tickets flagged.
- `DEN-new vesicle-routing-ledger` [standard-dev] — per-ticket token + escalation attribution; `vesicle status --ticket`. This is the evidence source for the deferred k=2 and HRR decisions. AC: routed job's tokens attributed within one session.
- Aider stays excluded; OpenCode deferred.
- **Dogfood hook**: Sessions 9–13 tickets pre-tagged and self-routing; Opus reads vesicle attribution each session start.

### Session 9 — Hermes Agent as fourth executor (user decision #1)
- `DEN-new hermes-install-sandboxed` [standard-dev] — install pinned Hermes (uv); **background review/cron/messaging gateways disabled, `skills.write_approval: true`** (upstream's own safety switches — adopt, don't patch); models exclusively via LiteLLM :4000; `immune scan` its skill tree. AC: headless run completes with daemon off; no process survives job exit.
- `DEN-new hermes-bouton-runner` [standard-dev, WRAP] — `pkg/agentjobs` runner (same shape as AGY runner): `bouton jobs submit --isolate --agent hermes`; capsule I/O; opt-in flag, never a default tier until benched. AC: one ticket lands end-to-end through hermes, gate clean.
- `DEN-new hermes-memory-isolation` [fast-impl] — Hermes memory scoped per-worktree (its own provider); `hook session-end` exports distilled facts one-way into Engram. AC: no direct Hermes writes to `~/.engram`; export idempotent.
- `DEN-new hermes-bench` [chore-local] — `velocity bench` matrix vs sonnet/3.8-flash on three ticket classes; routing decision recorded via `chronos decision`.
- **Dogfood hook**: Session 10 delegates ≥1 [fast-impl] ticket to the hermes runner.

## Stage E — Dual-protocol (Sessions 10–11)

### Session 10 — Scoped MCP over HTTP: adopt the official transport
- `DMCP-new adopt-mcp-gosdk-http` [standard-dev] — **session decision point: adopt the official MCP go-sdk streamable-HTTP transport + `.well-known` lazy-loading spec** (per the Mar-2026 MCP roadmap) in mcp-hub rather than extending the hand-rolled stdio server; keep stdio intact. Hand-roll only what the sdk genuinely lacks. AC: MCP client completes initialize+tools/list over HTTP; stdio unchanged.
- `DMCP-new lazy-manifest` [standard-dev] — `/.well-known` manifest = tool names + one-liners, per-tool schema on demand (collapses the ~69-tool upfront cost; raw plan's "72% context tax"). AC: initial payload schema-free; deferred tool callable after fetch.
- `DEN-new cortex-synapse-mcp-mount` [standard-dev] — mount `/synapses/<id>/mcp` on cortex :3456, binding existing `inter mcp serve --synapse --profile` scoping to HTTP. AC: two synapses serve disjoint tool sets; path escapes rejected.
- **Dogfood hook**: Session 11's agents connect over HTTP MCP instead of stdio.

### Session 11 — ACP blackboard + leases on rag-rs symbols (core vision — BUILD)
- **DEN-1009 (reuse)** mesh session store; **DEN-1011 (reuse)** blackboard JSONL + topic hub + SSE on cortex (generalize TelemetryHub per `docs/architecture/synapse-and-agent-mesh.md`); `IntentBroadcastEvent` first-class; **DEN-1013 (reuse)** `agent bus` CLI. Adopt the **A2A `agent-card.json` schema** for discovery instead of inventing one.
- `DRAG-new symbol-hash-lease-keys` [standard-dev, Rust/rstest] — rag-rs exposes stable `(file, symbol) → hash` from its existing symbol graph. AC: formatting-stable; <100ms warm on 10k symbols.
- **DEN-1010 (reuse, amended)** semantic leases + conflict notice + queue — keys are rag-rs symbol hashes (fallback whole-file), not core-rs (stub). `DCORE-new` tree-sitter successor = non-blocking backlog.
- **Dogfood hook**: Session 12's parallel saga forks register sessions, broadcast intents, hold leases.

## Stage F — Parallel probing & closeout (Sessions 12–13)

### Session 12 — `saga fork --mode parallel` + resolve composer (core vision — BUILD)
- `DEN-new saga-fork-parallel` [expert plan/standard-dev impl] — `--mode parallel --k 2`: k worktrees via `pkg/story/fork`; variant axes = harness (S6) and/or model (S8); probes scored by Pillar-1 gate; leases prevent clobbering. AC: concurrent forks, no conflicts on disjoint symbols; scores on ticket.
- `DEN-new saga-resolve-composer` [standard-dev] — deterministic Go interim: whole-branch winner by gate score + `git merge-file` for *disjoint-file* loser improvements only (overlapping symbols reported, never auto-merged). Semantic hunk compose deferred to git-rs (`DGIT-new` backlog linked to DEN-934, not duplicated). AC: identical inputs ⇒ identical composition.
- **Dogfood hook**: Session 13 runs ≥1 governance ticket through saga-parallel end-to-end.

### Session 13 — Governance closeout, cadence, retro (meiosis moved to DEN-833, 2026-11-03)
- ~~meiosis~~ → **pulled forward to DEN-833** (with pool lockstep linking) to unblock the MAPI workspace split; see the re-sequence section.
- `DEN-new cadence-scheduler` [fast-impl] — nightly `cognition dream`, weekly `egm-drift-report` across `docs/specs/`, 30d skill-staleness report; all on the cortex timeline.
- `DEN-new backlog-reconciliation-retro` [chore-local + expert review] — close superseded tickets; DEV_STATUS refresh; retro from vesicle data (tokens/tier, escalation rates, hermes bench, probe win-rates, **pool/meiosis usage from the MAPI use case**) → **decides the deferred items**: k=2 loop, HRR port, OpenCode onboarding, gbrain/Hindsight benchmark, OpenClaw relay, git-rs/core-rs successors.

---

## Deferred until evidence demands (never build speculatively)
k=2 propose-probe-compose (needs S8 cost data) · HRR trust-decay port (needs S5 gap evaluation) · full skill-curator lifecycle · OpenCode V2 executor · OpenClaw notification relay · gbrain/Hindsight archival tiers · git-rs semantic hunk compose (+ DEN-934 `--local-review`) · core-rs tree-sitter lease keys.

## Key files
- `pkg/inference/oxillama_server.go:274` — mock defect gating Stage A
- `pkg/agentjobs/fallback.go` — existing ladder S8 merges into
- `docs/architecture/synapse-and-agent-mesh.md` — lease/blackboard contract (S11)
- `docs/architecture/2026-10-quality-audit-execution-plan.md` — 13-pillar baseline this roadmap reuses
- `.github/dendrite/directives.toml` — gate parameters every AC must satisfy
- `research_*.md` → preserved under `docs/reports/research/` in S1

## Verification (end-to-end, per stage)
- A: immune fixtures pass/block correctly; `curl :8080/v1/chat/completions` returns real model output; `allostasis gate --since origin/develop` clean.
- B: `session resume` injects a bounded Engram snapshot; journal survives kill -9; `cognition dream --dry-run` reports.
- C: an egm-drafted spec gets 5 goldfish verdicts and a drift-gated `chronos finish`.
- D: `bouton jobs submit --isolate --agent hermes` completes a real ticket; `vesicle status --ticket` attributes tokens.
- E: MCP client works over `/synapses/<id>/mcp`; two agents exchange `IntentBroadcastEvent` and respect a lease.
- F: one ticket lands via `saga fork --mode parallel` + deterministic resolve; retro decides the deferred list.

