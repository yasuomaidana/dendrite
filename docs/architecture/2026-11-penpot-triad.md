# Penpot ↔ Code ↔ Plane: readiness audit and triad specification (2026-11)

> Ticket DEN-832 (spec) · implementation DEN-835 (service readiness), DEN-836 (triad glue), DMCP-2 (mcp-hub Penpot upstream), after DEN-433 (HTTP transport) and DEN-434 (symbol hashes). Penpot governs UI/UX design tokens, web components and storefront vector assets; the first consumer is the MAPI UI library (Plane project MAPIFE).

## Readiness audit (2026-11-03)

| Area | State | Gap |
|---|---|---|
| Penpot service | 4 containers healthy 2 days (`compose.penpot.yaml`: frontend :9000, backend :9001, postgres :5435, redis :6381); RPC answers on `/api/rpc/command/*` | backend has no healthcheck; `motor health penpot` probes only :9001 |
| API auth | `PENPOT_FLAGS = enable-prepl-server disable-email-verification disable-secure-session-cookies` — **`enable-access-tokens` is OFF**; only session-cookie auth | DEN-835 adds the flag + a PENPOT_ACCESS_TOKEN slot in `soma secrets ensure`; apply with `motor stop`+`start` (restart keeps stale env, DEN-441) |
| Exporter | **no exporter service**; `PENPOT_EXPORTER_URI` points at the backend | DEN-835 adds `penpot-exporter` (needed for asset/SVG export flows) |
| Penpot MCP server | **does not exist** — mcp-hub is a stdio-only Go library with no upstream registry; Penpot/Engram upstreams are plans (DMCP-1/2/3) | DMCP-2 after DEN-433; the official-Penpot-MCP deployment shape spike is DMCP-3 |
| Dendrite side | `pkg/ui/penpot`, `pkg/personalcontext/canvas`, `cortex sync` exist; `motor --verify-mcp` only parses a local fixture (mock) | real wiring lands in DEN-836 |

## The three edges

```
            (1) tokens/components/assets ->
  Penpot  <---------------------------------->  Project code (UI library)
   :9000    <- (2) impl status on symbol hash        |  ticket lock / pre-edit /
     ^                                               |  finish gate (exists today)
     | (3) frame links on [AC-n],                    v
     |     drift comments                     Plane v2 tickets :8090
     +----------------------------------------------+
```

### Edge 1 — Penpot → Code (prompt/context enrichment)
- mcp-hub registers Penpot as an **upstream** (DMCP-2) over the DEN-433 streamable-HTTP transport and re-exports `penpot_*` tools: design tokens, component hierarchy, layout specs, SVG assets.
- `hook pre-edit` / `dendrite_query_context` enrich a UI-tagged ticket's context with the frame/component the ticket links — the agent sees the design contract before editing.
- Auth: the DEN-835 access token, stored by `soma secrets ensure`, never printed.

### Edge 2 — Code → Penpot (implementation status back-sync)
- A post-merge job maps changed **exported symbols of the UI library** (rag-rs symbol hashes, DEN-434) to Penpot component metadata: prop contract and implementation status, via the RPC API.
- Keying on symbol hashes makes the sync formatting-stable and cheap: only components whose public surface changed are touched.

### Edge 3 — Penpot ↔ Plane
- Penpot frame/component links attach to tickets next to their `[AC-n]` lines (`chronos link`/`comment`).
- A drift check (design token or prop-contract mismatch between Penpot and the code) posts a comment on the ticket; excess drift can gate `chronos finish` the same way the EGM drift report does (S7).
- Ticket ↔ dev-environment gating already exists (ticket lock, `hook pre-edit`, finish gate) and closes the triangle.

## MAPI co-development playbook (use case, not design)

MAPI-internal decisions live in MAPIFE; Dendrite only provides the rails. The expected flow once the chain reaches each point:

1. **After DEN-833** — MAPIFE splits `frontend/` into Bun workspace packages (rendering lib, UI components, apps); `dendrite meiosis` registers them, `pool resolve` gives the dependency order. Precondition on the human side: land or park the ~123 uncommitted changes on `frontend/` branch `plan` first.
2. **After DEN-834** — MAPIFE scaffolds the print-layout Rust module with `--audit-library-reuse` (2-D rectangle packing / nesting crates from crates.io before any hand-written layout math), bridged by WASM or NAPI-RS under Bun.
3. **From DEN-427 on** — every MAPI+Dendrite co-dev session feeds Engram traces; ~2 weeks of these satisfy the S5 (DEN-428) soak.
4. **After DEN-434** — several agents work disjoint mapi packages concurrently under symbol leases.
5. **After DEN-836** — UI tickets carry Penpot frame links; prompts arrive design-token-enriched; implementation status flows back to Penpot.

## Out of scope
- Redesigning mapi's repo layout, package boundaries or business logic (user decision 2026-11-03: MAPI is a use case).
- Building a Penpot MCP server by hand if the official one fits — DMCP-3 decides the deployment shape first (adopt-vs-build rule).
- Kakebo projects stay paused in `dendrite store` and the vesicle scheduler while the MVP runs.
