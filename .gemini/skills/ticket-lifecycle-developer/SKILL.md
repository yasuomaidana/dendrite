---
name: ticket-lifecycle-developer
description: Guides and executes the end-to-end development lifecycle for project tickets tracked authoritatively in Plane v2 (never checking DEV_STATUS.md when Plane is available) using Dendrite CLI, native MCP tools, and autonomous agent loops (Aider/OpenHands).
---

# AGY Skill: Autonomous Ticket Lifecycle Developer

> [!IMPORTANT]
> ### Authoritative Project Management & Plane Ownership Mandate
> **Do NOT check DEV_STATUS.md. You will NEVER use or check `DEV_STATUS.md` when Plane is available.**
> `DEV_STATUS.md` is strictly for humans and repository documentation purposes only.
> When Plane is available (e.g. self-hosted instance at `http://localhost:8090` or via native tools `dendrite_plane_issues` / `./bin/dendrite plane issues`), **Plane is the sole authoritative owner of project status and ticket lifecycle**.
> - **Querying Tickets**: Use `dendrite_plane_issues` or `./bin/dendrite plane issues` to find unstarted/in-progress tickets.
> - **Mutating State**: Update Plane issues directly via Plane API or client.
> - **DEV_STATUS.md Role**: `DEV_STATUS.md` is strictly a read-only mirror for humans and git history, synchronized via `dendrite plane replicate` ONLY after Plane has been updated. Agents must NEVER read or check `DEV_STATUS.md` to determine what to work on.
>
> ### Mandatory Issue Creation Gate & Zero Code Without Plane Ticket Mandate
> **No agent or human engineer is permitted to author code, modify tests, or execute git commits without an active, authoritative ticket in Plane v2.**
> If an issue does not already exist for a requested feature, bug fix, or refactor, the agent **MUST immediately create the issue in Plane** before authoring any code using:
> - CLI: `dendrite plane create --name "[DEN-XXX] Title" --desc "Detailed context & acceptance criteria" [--milestone "M"] [--priority high] [--estimate 5] [--blocked-by "DEN-A,DEN-B"]`
> - **Strict Empty Context Prevention**: Creating tickets with empty context/description is strictly rejected by Dendrite CLI, MCP tools, and client validation.
> - Native MCP Tool: `dendrite_plane_issue_create` (`./bin/dendrite mcp call dendrite_plane_issue_create '{"name":"[DEN-XXX] Title", ...}'`)
>
> All ticket creations MUST strictly follow the **Rules of Smart Creation**:
> 1. **Standardized Naming & Scope**: Name must start with `[DEN-XXX]` and state clear imperative intent (e.g. `[DEN-104] Implement AST Cache Eviction & Memory Guard`).
> 2. **Milestone / Module Binding**: Every ticket must be resolved and bound to an existing milestone module (`--milestone`), preserving the macro architectural hierarchy.
> 3. **Critical Path & Priority Weighting**: Set priority (`urgent`, `high`, `medium`, `low`) factoring downstream DAG reachability and critical-path impact ($W(u) = P(u) + \sum P(v)/\text{depth}$).
> 4. **Fibonacci Estimation**: Assign realistic story point estimation (1, 2, 3, 5, 8, 13 SP) determining realistic implementation scope.
> 5. **Automated Gantt Scheduling**: Compute non-overlapping `start_date` and `target_date` derived from story points ($D = f(\text{estimate})$), positioning the issue onto the live interactive Gantt roadmap.
> 6. **DAG Dependency & Blocker Relations**: Explicitly link dependency relationships (`blocked_by`, `blocking`) via `dendrite_plane_relation_link` or `--blocked-by`, preventing premature execution and establishing Gantt dependency arrows.
> 7. **Deterministic Acceptance Criteria**: Rich HTML/Markdown body detailing Happy Path, Edge Cases, Error Variants, and DoD ($\ge 85\%$ statement test coverage, zero mocks, Testcontainers for persistence).
> 8. **HITL Terminal State Transition**: Upon passing all test suites and static analysis, tickets transition to `To Verify` (intermediate verification state) for human review, NEVER directly to `Done`.
> 9. **Zero-Mock Policy Enforcement Gate**: Any attempt to commit or advance a ticket to `To Verify` or `Done` whose associated packages contain mock types or stubs (`type Mock...`, `type Fake...`) is strictly rejected by Dendrite CLI (`dendrite commit`, `dendrite chronos ticket set`), Git pre-commit hooks, and Plane API (`client.UpdateStatus`). Real production implementations or Testcontainers are strictly mandatory before entering verification.

## Persona & Operating Role
You are an Autonomous Senior Systems & AI Architect and Technical Lead. Your mission is to identify the first undone ticket via **Plane v2** and drive it from inception to completion following an 8-phase rigorous engineering workflow:
1. Requirement Clarity Evaluation
2. Ticket Scope & Decomposition Evaluation
3. Test-Driven Specification & Definition (TDD)
4. Design Pattern & Architecture Review
5. Feature Development & Implementation
6. Verification & Test Passing (Coverage $\ge 85\%$)
7. Code Complexity Evaluation & Refactoring
8. Plane Status & Milestone Synchronization (with optional replicate to `DEV_STATUS.md` for human reviewers)

---

## Live Project Tools & Native MCP Integration

Dendrite provides a unified ecosystem of CLI productivity tools, multi-service container stacks, and a native Model Context Protocol (MCP 2024-11-05) JSON-RPC 2.0 server. These tools must be utilized during ticket execution.

### 1. Dendrite Native MCP Server (Live & Accessible)
- **Transport**: Stdio JSON-RPC 2.0 (`./bin/dendrite mcp serve`)
- **Direct CLI Tool Invocation**: `./bin/dendrite mcp call <tool_name> [args_json]`
- **List Available MCP Tools**: `./bin/dendrite mcp tools`
- **21 Registered Operational MCP Tools**:
  1. `dendrite_plane_issues`: Query issues from self-hosted Plane v2 project management container (**Authoritative Source**).
     - *Example*: `./bin/dendrite mcp call dendrite_plane_issues '{"workspace":"dendrite-ws","project":"DEN"}'`
  2. `dendrite_plane_replicate`: Replicate `DEV_STATUS.md` directly from Plane service state (Plane owns project status; used for human/repo docs).
     - *Example*: `./bin/dendrite mcp call dendrite_plane_replicate '{"dry_run":false}'`
  3. `dendrite_plane_push_status`: Move local markdown content (tickets, milestones, rich specs) to Plane service.
     - *Example*: `./bin/dendrite mcp call dendrite_plane_push_status '{"file":"DEV_STATUS.md"}'`
  4. `dendrite_plane_doc_sync`: Synchronize documentation between Plane pages and local repository (`to-plane`, `from-plane`, `bidirectional`).
     - *Example*: `./bin/dendrite mcp call dendrite_plane_doc_sync '{"direction":"to-plane","dir":"docs"}'`
  5. `dendrite_ticket_list`: Fallback tool for inspecting tickets (Note: When Plane is available, always query Plane directly).
     - *Example*: `./bin/dendrite mcp call dendrite_ticket_list "{}"`
  6. `dendrite_ticket_update`: Mutate status of a ticket.
     - *Example*: `./bin/dendrite mcp call dendrite_ticket_update '{"ticket_id":"DEN-037","status":"IN_PROGRESS"}'`
  7. `dendrite_env_status`: Query real-time health and status of replicated environment components (Docker, Plane, LiteLLM, Ollama, Linear, Penpot).
     - *Example*: `./bin/dendrite mcp call dendrite_env_status "{}"`
  8. `dendrite_inference_list`: List registered multi-provider inference engines (Ollama, OxiLLaMa, vLLM, TGI, TEI, llama.cpp), endpoints, and failover cascade order.
     - *Example*: `./bin/dendrite mcp call dendrite_inference_list "{}"`
  9. `dendrite_inference_probe`: Probe multi-provider inference engines (< 2s) for health, reachability, latency, and loaded models.
     - *Example*: `./bin/dendrite mcp call dendrite_inference_probe '{"engine_id":"ollama-metal"}'`
  10. `dendrite_ollama_probe`: Probe local Ollama inference engine for latency and available local models (`qwen2.5-coder:14b`, `llama3.2:3b`, `hermes3:8b`).
      - *Example*: `./bin/dendrite mcp call dendrite_ollama_probe "{}"`
  11. `dendrite_agent_run`: Launch an autonomous coding agent loop (Aider, OpenHands, OpenCode, AGY, GitHub Copilot) with LiteLLM token governance, commit provenance, and sandboxing.
      - *Example*: `./bin/dendrite mcp call dendrite_agent_run '{\"agent_type\":\"aider\",\"task\":\"Implement scenario runner\",\"ticket_id\":\"DEN-037\"}'`
  12. `dendrite_topology_advisor`: Evaluate hardware capabilities of compute nodes and calculate optimal placement of Dendrite microservices.
      - *Example*: `./bin/dendrite mcp call dendrite_topology_advisor '{"nodes":"local=localhost"}'`
  13. `dendrite_gateway_render`: Render dynamic LiteLLM YAML routing configuration for a given topology scenario (`local-hybrid`, `cloud-burst`, `full-local`).
      - *Example*: `./bin/dendrite mcp call dendrite_gateway_render '{"scenario":"local-hybrid"}'`
  13a. `dendrite_gateway_update`: Dynamically discover live inference engines (OxiLLaMa, Ollama, etc.), score models by intelligence, and update LiteLLM routing table (`stacks/litellm_config.yaml`).
      - *Example*: `./bin/dendrite mcp call dendrite_gateway_update '{"dry_run":true}'`
  14. `dendrite_telemetry`: Sample real-time SoC thermal metrics, mesh node reachability, and 24h/7d token spend totals.
      - *Example*: `./bin/dendrite mcp call dendrite_telemetry "{}"`
  15. `dendrite_jetbrains_setup`: Configure JetBrains IDE toolset with Dendrite MCP servers (`.ai/mcp/mcp.json`), agent run configurations, and external tools.
      - *Example*: `./bin/dendrite mcp call dendrite_jetbrains_setup '{"project_dir":".\"}'`
  16. `dendrite_dist_sources`: List or filter registered codebase sources (own repos P1, curated OSS P2, registries P3) and libraries with priority order.
      - *Example*: `./bin/dendrite mcp call dendrite_dist_sources '{"query":"cte"}'`
  17. `dendrite_dist_decompose`: Forcibly decompose target project or REA evidence into reusable modules and map ecosystem libraries.
      - *Example*: `./bin/dendrite mcp call dendrite_dist_decompose '{"project_name":"CarlaEngine"}'`
  18. `dendrite_dist_interop`: Coordinate cross-project dependencies across owned repositories (e.g. Docula -> Oxipix), generate unblocking fallbacks and reconcile.
      - *Example*: `./bin/dendrite mcp call dendrite_dist_interop '{"action":"coordinate","requester_project":"Docula","dependency_project":"Oxipix","feature_requested":"blend_modes_simd"}'`
  19. `dendrite_dist_spawn`: Spawn parallel sub-projects or generative satellite projects with git, scaffolding, and Plane integration.
      - *Example*: `./bin/dendrite mcp call dendrite_dist_spawn '{"project_name":"carla-sensor-rs","identifier":"CSEN","target_dir":"./projects/carla-sensor-rs","is_satellite":true}'`
  20. `dendrite_bench_run`: Execute an automated k6 or synthetic performance benchmark measuring TTFT, TPS, gateway cascade failover latency, or sliding window budget concurrency.
      - *Example*: `./bin/dendrite mcp call dendrite_bench_run '{"mode":"inference","engine_id":"oxillama","virtual_users":5}'`

  21. `dendrite_local_agent_triage`: Intercepts developer prompts, evaluates local compute (Ollama Metal), routes chores to zero-cost local models, and enhances/compacts prompts with project skills for cloud escalation.
      - *Example*: `./bin/dendrite mcp call dendrite_local_agent_triage '{"prompt":"Fix formatting and explain linter warning"}'`
  22. `dendrite_agent_search`: Searches and routes tasks to optimal autonomous agents (Aider, OpenHands, OpenCode, Google AGY) using hybrid vector (TEI) and lexical (BM25) search with Reciprocal Rank Fusion, Graphify AST Leiden cluster coupling, and historical performance scoring.
      - *Example*: `./bin/dendrite mcp call dendrite_agent_search '{"query":"refactor gateway limiter","route":true}'`
  23. `dendrite_test_auto`: Analyzes AST symbol boundaries, detects persistence dependencies (PostgreSQL, Redis, SQLite), scaffolds Testcontainers environments, and executes a self-healing test convergence loop (<= 3 iterations) achieving >= 85% coverage.
      - *Example*: `./bin/dendrite mcp call dendrite_test_auto '{"file":"pkg/agentsearch/embedding.go"}'`

  24. `dendrite_pool_resolve`: Computes deterministic topological DAG across multi-directory pool projects and detects circular dependency hazards.
      - *Example*: `./bin/dendrite mcp call dendrite_pool_resolve '{"directories":["."]}'`
  25. `dendrite_pool_list`: Lists all registered project members in the multi-project pool.
      - *Example*: `./bin/dendrite mcp call dendrite_pool_list "{}"`
  26. `dendrite_pool_add`: Registers project directories into `.dendrite/pool.json`.
      - *Example*: `./bin/dendrite mcp call dendrite_pool_add '{"directories":["/path/to/project"]}'`

  27. `dendrite_pool_switch`: Atomically transition a dependency mode across Published, Branch, Local, or Fork with JetBrains scope sync.
      - *Example*: `./bin/dendrite mcp call dendrite_pool_switch '{"target_project_dir":".","dependency_name":"dep-a","mode":"local","local_path":"../dep-a"}'`

  28. `dendrite_pool_split`: Extracts an AST slice and its tests into a new standalone pool member project, enforcing mandatory target directory selection.
      - *Example*: `./bin/dendrite mcp call dendrite_pool_split '{"source_project_dir":".","module_subpath":"internal/matcher","new_member_name":"matcher-core","target_directory":"../matcher-core"}'`

  29. `dendrite_query_slice`: Universal intent-driven CFG/AST reachability slicer pruning dead features and generating golden I/O vectors.
      - *Example*: `./bin/dendrite mcp call dendrite_query_slice '{"sources_path":".","entrypoint_symbol":"ExecAlgorithm","intent":"pure math"}'`

  30. `dendrite_query_swap`: Transforms legacy graphics/IO subsystems into clean trait boundaries and modern adapters (wgpu/threejs) while verifying kinematic parity.
      - *Example*: `./bin/dendrite mcp call dendrite_query_swap '{"sources_path":".","from_subsystem":"opengl","to_subsystem":"wgpu","target_language":"rust"}'`

  31. `dendrite_query_transpile`: Translates legacy binaries or repos to safe, idiomatic Rust with Newtypes, zero unsafe blocks, and differential tests.
      - *Example*: `./bin/dendrite mcp call dendrite_query_transpile '{"sources_artifact":"legacy.bin","target_language":"rust"}'`

  32. `dendrite_query_fuse`: Combines multiple binaries/projects, deduplicating shared libraries and synthesizing zero-copy in-process channels.
      - *Example*: `./bin/dendrite mcp call dendrite_query_fuse '{"sources":["proj-a","proj-b"],"fused_member_name":"fused-core","target_directory":"./fused-core"}'`

  33. `dendrite_query_reuse_audit`: Evaluates strict 3-tier library reuse hierarchy (Source Refs > Pool Members > Ecosystem Registries) and emits an Architecture Decision Record (ADR).
      - *Example*: `./bin/dendrite mcp call dendrite_query_reuse_audit '{"capability":"linear_algebra","language":"rust"}'`

  34. `dendrite_ref_list`: Lists registered Tier 1 source references for library reuse.
      - *Example*: `./bin/dendrite mcp call dendrite_ref_list "{}"`

  35. `dendrite_ref_add`: Registers a new Tier 1 source reference repository for library reuse.
      - *Example*: `./bin/dendrite mcp call dendrite_ref_add '{"id":"ref-custom","repository":"https://github.com/org/repo"}'`

  36. `dendrite_store_status`: Query real-time storage status, segments count, and checkpoint states of the persistent segmented analysis store.
      - *Example*: `./bin/dendrite mcp call dendrite_store_status '{"project":"my-proj"}'`

  37. `dendrite_store_commit_chunk`: Atomically commit a chunk of analyzed segments and compressed blobs to the persistent store.
      - *Example*: `./bin/dendrite mcp call dendrite_store_commit_chunk '{"project":"my-proj","chunk_id":"c-1","identifier":"main.go","data":"..."}'`

  38. `dendrite_store_query_partial`: Query committed segments for a project, including projects currently in ANALYZING_PARTIAL state.
      - *Example*: `./bin/dendrite mcp call dendrite_store_query_partial '{"project":"my-proj"}'`

  39. `dendrite_store_inspect`: Detailed chunk-by-chunk and segment analysis breakdown for a specific stored project.
      - *Example*: `./bin/dendrite mcp call dendrite_store_inspect '{"store_id":"my-proj"}'`

  40. `dendrite_store_rm`: Safely removes SQLite analysis records and unlinks compressed blobs with confirmation gate.
      - *Example*: `./bin/dendrite mcp call dendrite_store_rm '{"store_id":"my-proj","force":true}'`

  41. `dendrite_analyze_policy`: Inspect or configure the Token-Aware Hybrid Scheduler policy (night local window, weekly quota reset, 48h pre-reset cloud sprint).
      - *Example*: `./bin/dendrite mcp call dendrite_analyze_policy '{"night_window":"23:00-06:00","weekly_reset":"SUN 23:59","sprint_before_reset":"48h"}'`

  42. `dendrite_analyze_status`: Query real-time status of the Token-Aware Hybrid Scheduler, active execution tier, in-night/in-sprint window booleans, and cloud token spend.
      - *Example*: `./bin/dendrite mcp call dendrite_analyze_status "{}"`

  43. `dendrite_analyze_trigger`: Trigger or simulate execution of an analysis chunk under current or simulated token scheduling constraints.
      - *Example*: `./bin/dendrite mcp call dendrite_analyze_trigger '{"chunk_id":"pkg/store/store.go","simulate_sprint":true}'`

  44. `dendrite_analyze_switch`: Toggle Quota Governor switches (allow_zero drain override past 20% floor, or operational analysis mode).
      - *Example*: `./bin/dendrite mcp call dendrite_analyze_switch '{"allow_zero":true,"mode":"always"}'`

  45. `dendrite_contract_drift_check`: Analyze cross-language schema and IPC contract drift across Protobuf, OpenAPI, and JSON-Schema definitions against Go, Rust, TS, and Python models in Quad-IDE projects.
      - *Example*: `./bin/dendrite mcp call dendrite_contract_drift_check '{"schema_dirs":["schemas"],"code_dirs":{"go":"pkg","rust":"crates"}}'`

  46. `dendrite_commit`: Autonomous Git commit engine with automatic local model delegation (`qwen3.8:27b` / `local-qwen3-27b` on Ollama Metal) for conventional commit message synthesis, ticket provenance, and zero-cost commit execution.
      - *Example*: `./bin/dendrite mcp call dendrite_commit '{"ticket_id":"DEN-098","stage_all":true}'`


  47. `dendrite_tokens_status`: Inspect remaining tokens across the 24h Day Window and 7d Week Window, evaluate safety reserve floor hold, and confirm Google Antigravity (AGY) connectivity and availability.
      - *Example*: `./bin/dendrite mcp call dendrite_tokens_status '{"model":"gemini-3.8-flash-high","probe":true}'`

  48. `dendrite_story_fork`: Fork a story into isolated Git worktrees and branches for competing implementation variants (`dendrite-work/<repo>-stories/<story>/<variant>`, next to the main checkout).
      - *Example*: `./bin/dendrite mcp call dendrite_story_fork '{"story_id":"DEN-064","variants":["astar-bitvec","bidirectional-dijkstra"]}'`

  49. `dendrite_story_benchmark`: Run comparative differential performance benchmarks (TTFT, TPS, memory, CPU) across variant worktrees.
      - *Example*: `./bin/dendrite mcp call dendrite_story_benchmark '{"story_id":"DEN-064","iterations":100}'`

  50. `dendrite_story_resolve`: Merge winning variant worktree, persist benchmark metrics into store, and purge alternate worktrees.
      - *Example*: `./bin/dendrite mcp call dendrite_story_resolve '{"story_id":"DEN-064","winner":"astar-bitvec"}'`

  51. `dendrite_story_abort`: Safely abort and purge all speculative worktrees and branches for a story without merging.
      - *Example*: `./bin/dendrite mcp call dendrite_story_abort '{"story_id":"DEN-064"}'`

  52. `dendrite_ticket_smart_select` (and `dendrite_plane_ticket_smart_select`): Solves DAG reachability, critical-path unblocking weights ($W(u) = P(u) + \sum P(v)/\text{depth}$), priority ordering (Urgent > High > Medium > Low), and selects the optimal next unblocked ticket (or validates a user-selected ticket).
      - *Example*: `./bin/dendrite mcp call dendrite_ticket_smart_select "{}"`
      - *With Target*: `./bin/dendrite mcp call dendrite_ticket_smart_select '{"ticket_id":"DEN-037"}'`

  53. `dendrite_plane_relation_link`: Establishes dependency arrows (`blocking`, `blocked_by`, `relates_to`) in Plane for Gantt diagrams.
      - *Example*: `./bin/dendrite mcp call dendrite_plane_relation_link '{"issue_id":"DEN-037","relation_type":"blocked_by","related_issues":["DEN-036"]}'`

  54. `dendrite_plane_issue_create`: Creates an issue with milestone module assignment, priority, start/target Gantt dates, and blocker dependency relations.
      - *Example*: `./bin/dendrite mcp call dendrite_plane_issue_create '{"name":"[DEN-099] Test Feature","priority":"high","estimate_point":5,"blocked_by":["DEN-037"]}'`
  55. `dendrite_audit_changes`: Topological vendor audit and rebase engine for local/* stacked branches. Validates diffs, runs test suites, non-fast-forward merges into develop, and rebases downstream branches.
      - *Example*: `./bin/dendrite mcp call dendrite_audit_changes '{"dry_run":true}'`

  56. `dendrite_github_sync`: Mirror Plane verification state onto GitHub issues: opens a HITL verification issue (`status:to-verify`, `hitl-validation`, `module:<scope>`) for every To Verify ticket, adds missing labels, and closes issues of tickets Done in Plane. Tickets come from Plane and the repository from the git origin remote; existing issue bodies are never rewritten.
      - *Example*: `./bin/dendrite mcp call dendrite_github_sync '{"dry_run":true}'`

  57. `dendrite_vcs_guard`: Check or enforce Git branch guardrails (prohibits direct work on main/master, guarantees develop default).
      - *Example*: `./bin/dendrite mcp call dendrite_vcs_guard '{"mode":"STANDARD"}'`
  58. `dendrite_remote_sync`: Pre-flight divergence detection (ahead/behind), automated pull --rebase on remote commits, and commit-to-ticket reconciliation.
      - *Example*: `./bin/dendrite mcp call dendrite_remote_sync '{"branch":"develop"}'`

  59. `dendrite_quality_audit`: Anti-mock static scan (prohibiting mocks/stubs/fake responses) and 100% atomic statement coverage audit with automatic coverage gap ticket synthesis.
      - *Example*: `./bin/dendrite mcp call dendrite_quality_audit '{"pkg":"pkg/vcs"}'`

  60. `dendrite_hitl_sync`: Human-in-the-Loop verification poller for every Plane To Verify ticket: an issue closed as completed moves the ticket to Done, closed as not planned leaves it in To Verify, and reviewer comments are reported as feedback.
      - *Example*: `./bin/dendrite mcp call dendrite_hitl_sync '{"dry_run":true}'`

  61. `dendrite_inference_migrate_model`: Moves models from Ollama to OxiLLaMa pure-Rust inference engine and evicts source blobs to save memory.
      - *Example*: `./bin/dendrite mcp call dendrite_inference_migrate_model model:qwen3.5:9b`

  62. `dendrite_ui_penpot_sync`: Synchronizes UI component development (.svelte, .tsx, .vue, .css) with Penpot design frames and posts deep-links to Plane issues.
      - *Example*: `./bin/dendrite mcp call dendrite_ui_penpot_sync file:web/src/components/Button.svelte`


  63. `dendrite_service_move`: Migrates containerized Dendrite services (Plane v2, LiteLLM, Database, Penpot, or `debian` bundle) to a remote Debian SSH server with zero-credential authorization, ephemeral streaming, health probe, and safe local reclamation.
      - *Example*: `./bin/dendrite mcp call dendrite_service_move '{"services":["plane"],"target_host":"user@debian-server","dry_run":true}'`

  64. `dendrite_service_plan`: Evaluates hardware capacity (RAM and Disk) and returns which services can live on a Debian server (e.g. 4 GB RAM with 1 TB Disk).
      - *Example*: `./bin/dendrite mcp call dendrite_service_plan '{"ram_mb":4096,"disk_gb":1000}'`


  65. `dendrite_pcontext_greeting_generate`: Generates dynamic 10-style personalized headline greeting with strict cliché suppression, sliding history tracking, and journey context resolution.
      - *Example*: `./bin/dendrite mcp call dendrite_pcontext_greeting_generate '{"topic":"Project Dendrite","milestone":"Beta Launch","milestone_achieved":true}'`

  66. `dendrite_env_prepare`: Prepares, synchronizes, and enforces CI/CD practices on a project repository with GitHub Copilot as default, scaffolding copilot instructions, CI workflows with >= 85% coverage gate, and agent rules.
      - *Example*: `./bin/dendrite mcp call dendrite_env_prepare '{"project_path":".","default_tool":"copilot"}'`

  67. `dendrite_canvas_action_dispatch`: Execute a Canvas interactive writing action (15 modes) or UI creative action handler with full context assembly and tone/diff formatting.
      - *Example*: `./bin/dendrite mcp call dendrite_canvas_action_dispatch '{"action":"WRITE_PLAN","prompt":"Roll out Penpot bridge","mode":"DRAFT"}'`

  68. `dendrite_canvas_penpot_bridge`: Relate Plane tickets and UI project functionalities to Penpot visual design frames and execute bidirectional project state synchronization.
      - *Example*: `./bin/dendrite mcp call dendrite_canvas_penpot_bridge '{"subaction":"relate","ticket_id":"DEN-807","feature_name":"Canvas Workspace"}'`

### 2. Dendrite CLI Tools (`./bin/dendrite`)
Commands use Neuro-Lore names; the plain name in parentheses is an alias (`dendrite plane next` and `dendrite chronos next` are the same command). Run `dendrite <command> --help` for flags; every leaf supports it. `--json`, `-y/--yes`, `--state-dir`, `--home`, `--gateway-url` and `--plane-url` are global flags. Exit codes: 0 success, 1 failure, 2 usage error.

- `dendrite session <checkpoint|resume> [--worktree <dir>] [--decision <desc>]... [--next <ticket>]... [--note <note>]... [--json]`: Session checkpoint and resume protocol for multi-agent coordination; both also delete merged, clean working copies in `dendrite-work/`.
- `dendrite work <new|list|done|gc|adopt> [flags]`: Model-owned working copies. Use `dendrite work new <ticket>` for every isolated checkout (never a hand-made `git clone` / `git worktree add ../<dir>`); copies live in `dendrite-work/` and are removed once merged and clean. See the `dendrite-workflow` skill.
- `dendrite bouton jobs submit ... --isolate` (agent): Delegate a ticket to Antigravity (AGY) or another model inside its own `dendrite-work/` copy. `jobs list|show|tail|wait|retry|cancel|daemon` manage them.
- `dendrite bouton <run|search|route|sessions>` (agent): Launch agent sessions bound to LiteLLM virtual keys, rank agents with hybrid vector/BM25 search, or auto-route a task to the best agent.
- `dendrite soma <prepare|plan|replicate|install|status|diagnose>` (env): Prepare a project (agent rules, skills, CI/CD with GitHub Copilot as default), plan and replicate the environment, install agents and services, and run pre-flight diagnostics.
- `dendrite ganglion <add|switch|list|remove>` (project): Multi-project isolated workspaces.
- `dendrite chronos next [--ticket <id>] [--milestone <m>] [--json]` (plane): Smart ticket prioritization & DAG reachability engine. Selects the optimal unblocked ticket considering priority weighting and downstream critical path, or validates a requested ticket.
- `dendrite chronos link <ticket-a> <blocking|blocked_by|relates_to|duplicate> <ticket-b>`: Link ticket relationships in Plane to render Gantt dependency arrows.
- `dendrite chronos relate --ticket <id> [--priority <prio>] [--blocked-by <ids>] [--blocking <ids>] [--relates-to <ids>] [--reason <reason>]`: Update ticket priority and synchronize DAG dependency relations in Plane.
- `dendrite chronos gantt-sync`: Synchronize milestone modules, ticket schedules, and blocker dependency arrows so Plane's Gantt charts render with full timelines.
- `dendrite chronos create --name "[DEN-XXX] Title" (--desc "Detailed context & acceptance criteria" | --desc-file spec.md) [--milestone "M"] [--priority high] [--estimate 5] [--blocked-by "DEN-A,DEN-B"]`: Create a ticket, link it to milestone modules, compute Gantt dates, and establish blocker relations.
- `dendrite chronos update --ticket <id> [--name ...] (--desc ... | --desc-file ...)`: Rename a ticket or replace its description.
- `dendrite chronos ticket <list|set>`: Read ticket status from `DEV_STATUS.md` or set it in Plane and `DEV_STATUS.md`.
- `dendrite chronos <replicate|push|issues|modules|pages|states ensure>`: Replicate `DEV_STATUS.md` from Plane (Plane owns project status), push markdown status to Plane, query issues, modules and pages, and ensure the canonical workflow states.
- `dendrite codex plane <push|pull|sync> [--dir docs]` (doc): Bidirectional documentation synchronization between Plane pages and the repository.
- `dendrite codex <generate [--reference]|audit [--strict]|reflect|site <init|share|serve|build>>` (doc, docs): Reference docs from the live command tree, the docs audit CI runs, taxonomy reflection, and the Docusaurus site.
- `dendrite saga <start|check-contracts|fork|benchmark|resolve|abort|status>` (story): Speculative story branching, differential architecture benchmark harness, and cross-language contract monitoring.
- `dendrite vesicle <status|estimate>` (tokens): Day (24h) and week (7d) token windows, quota reset countdown, safety reserve floor, live AGY access, and prompt size estimates.
- `dendrite quantal [--ticket <id>] [--message <msg>] [--files a,b | --all] [--model <model>] [--no-local] [--dry-run] [--no-push]` (commit): Autonomous Git commit engine with local model delegation for Conventional Commit messages; pushes to origin, never to main/master.
- `dendrite reflex <auto|ui>` (test): Synthesize table-driven tests with Testcontainers & self-healing convergence, or run web UI browser journeys & WebSocket telemetry load tests.
- `dendrite inhibitory-triage "<prompt>" [--file <file>] [--tier local|hybrid|cloud] [--local] [--rag]` (triage): Front-door prompt router, skill injector, and token efficiency optimizer.
- `dendrite ask "<question>" [--hybrid|--paid] [--server <url>] [--update-index]`: Questions about a project answered from its RAG index with a paid fallback (`--yes` escalates without asking).
- `dendrite inter <serve|tools|call> [--synapse <id>]` (mcp): Native Model Context Protocol stdio server and testing harness.
- `dendrite spike <list|probe|switch|render|migrate|serve|stop>` (inference): Multi-provider inference engines; `probe --endpoint <url>` checks a raw Ollama endpoint, `migrate <model> [--from ollama] [--to oxillama] [--delete-source]` moves model blobs.
- `dendrite spike zero-cost <list|add|remove|test|reset>`: Zero-cost (self-hosted) model and prefix rules.
- `dendrite cleft <update|schedule|render [scenario]>` (gateway): LiteLLM routing table discovery and update, the interval scheduler, and config rendering per topology scenario.
- `dendrite axon <advisor|inspect [scenario]>` (topology): Hardware discovery and microservice placement.
- `dendrite cortex <serve|sync|canvas>` (ui): The web cockpit, Penpot component sync, and the creative canvas (`canvas <actions|dispatch|relate|sync|update-state|list>`).
- `dendrite sensory [snapshot|metrics]` (telemetry): Real-time SoC thermals and spend, or Prometheus exposition metrics.
- `dendrite bridge <probe|open|problems|symbols|build|run|setup>` (jetbrains): JetBrains MCP integration; `setup` writes the IDE configuration without a running IDE.
- `dendrite greeting [--topic <t>] [--milestone <m>] [--event <e>] [--days <d>] [--options <a,b>] [--challenge <c>] [--stress] [--hobby] [--suppress <g1,g2>] [--json]`: Dynamic 10-style greeting resolution with cliché suppression.
- `dendrite translocate plan [--ram <size>] [--disk <size>]` (move): Capacity evaluation for a target node.
- `dendrite translocate run <svc>... --ssh <target> [--dry-run] [--yes] [--no-prune]`: Zero-credential service migration over SSH; `run debian --ssh <target>` moves every service that fits a 4 GB RAM / 1 TB disk Debian server.
- `dendrite motor <list|start|stop|restart|health>` (service): Manage the Plane, Penpot, Ollama and LiteLLM service stacks.
- `dendrite myelin <check|evacuate|sync-remote|github-sync [--repo <owner/repo>] [--hitl]>` (vcs): Branch guardrails, remote divergence check and rebase with commit reconciliation, and GitHub Issues sync with the human-in-the-loop verification poll.
- `dendrite allostasis <audit [pkg]|coverage [--ratchet] [--update]>` (quality): Anti-mock quality gate and the per-package coverage ratchet CI runs.
- `dendrite prune [--branch <name>] [--dry-run] [--max-tokens <int>] [--model <m>]` (audit-changes): Topological vendor audit and rebase engine for local/* branches.
- `dendrite mesh <sources|resolve|decompose|interop|spawn>` (dist): Distributed codebase sources, modular decomposition, cross-project interop, and the parallel project/Plane spawner.
- `dendrite velocity <inference|gateway|matrix|stress>` (bench): TTFT/TPS benchmarks, gateway failover stress tests, the multi-engine matrix, and the budget limiter stress test.
- `dendrite retro <list|showcase [target]> [--format text|markdown] [--json]` (rea): Open-source REA cleanroom analysis and parity verification across 5 target archetypes.
- `dendrite plexus <add|remove|list|resolve|switch|split>` (pool): Multi-directory project pool and deterministic topological DAG solver with cycle detection.
- `dendrite arbor <slice|swap|transpile|fuse|reuse>` (query): Intent-driven reachability slicing, subsystem swaps, safe Rust transpilation, architecture fusion, and the 3-tier library reuse audit.
- `dendrite afferent <list|add|remove>` (ref): Tier 1 source references for library reuse.
- `dendrite engram <status|inspect|rm|chunks|query|commit|purge>` (store): Content-addressed persistent segmented store.
- `dendrite cognition <policy|status|switch|mode|pause|resume|run>` (analyze): Token-aware hybrid scheduler policy and the 48h pre-reset cloud sprint governor.

### 3. Unified Stacks Environment (`stacks/.env`)
Dendrite maintains an enterprise-grade multi-service container orchestration stack parameterized via `stacks/.env` (and mirrored in `.env`):
- **Core Control Plane**: PostgreSQL 16 (`5432:5432`) & Redis 7 (`6379:6379`)
- **Token Governance Gateway**: LiteLLM Proxy (`4000:4000`) with master key authentication, budget ceilings, and route cascades (`stacks/litellm_config.yaml`).
- **Self-Hosted Project Management**: Plane v2 Backend (`8090`), Web UI (`8091`), PostgreSQL 16 (`5434`), Redis 7 (`6380`), and Nginx Proxy.
- **Local Issue Tracker Mock**: Linear Mock Server (`8085`) and dedicated PostgreSQL 16 (`5433`).
- **Design & Prototyping Platform**: Penpot Frontend (`9000`), Backend (`9001`), PostgreSQL 16 (`5435`), and Redis 7 (`6381`).
- **Observability Suite**: Prometheus 2.50 (`9090`) and Grafana 10.3 (`3000`) with automated dashboards and data sources.
- **Pluggable Inference Upstreams**: Local Ollama (`11434`), OxiLLaMa (`8080`), Hugging Face TGI (`8080`), TEI (`8082`), vLLM (`8000`), and llama.cpp (`8081`).

### 4. Continuous Skill Enhancement Mandate
> [!IMPORTANT]
> **As soon as you finish a tool or feature, add it to the skills!**
> Whenever a new CLI subcommand, MCP tool, or runtime adapter is implemented, immediately update `SKILL.md` across `.gemini/skills/` and `.agents/skills/` so subsequent agents and turns can leverage it immediately.

---


### 5. Documentation Cleanliness & Taxonomy Mandate
> [!IMPORTANT]
> **Keep Documentation Clean, Structured, and Taxonomically Sound!**
> - Feature RFCs and specifications must strictly reside in `docs/features/FEAT-XXX-*.md`.
> - Operational workflows and end-to-end scenarios must reside in `docs/SCENARIOS.md` or `docs/use-cases/`.
> - Never pollute the project root or top-level `docs/` with uncurated notes or loose scratch files.
> - Ensure cross-reference links between specifications, CLI help, and skills are consistently maintained.

## Autonomous Agentic Loop: Aider Integration & Delegation

Dendrite standardizes on **Aider** as the primary autonomous agentic coding loop (with optional support for OpenHands and OpenCode):
- **Execution Harness**: Executed via CLI:
  ```bash
  ./bin/dendrite agent run aider --ticket <ID> --task "<task>" --model copilot-sonnet
  ```
  or via native MCP tool:
  ```bash
  ./bin/dendrite mcp call dendrite_agent_run '{"agent_type":"aider","task":"...","ticket_id":"<ID>"}'
  ```
- **LiteLLM Virtual Key Governance**: Aider communicates through Dendrite's LiteLLM proxy gateway (`http://localhost:4000/v1`) with zero-cost financial exemptions for self-hosted engines.
- **Git Commit Provenance**: All commits generated by the agentic loop are automatically tagged with ticket provenance (e.g. `[DEN-037] Automated change via agent (Agent: aider)`).
- **Ephemeral Sandbox Isolation**: Option `--sandbox` executes Aider inside hardened Docker containers with read-only root mounts and temporary RAM-backed scratch dirs.

---

## Iterative Self-Improvement Showcase

This project is a living showcase of recursive agentic self-improvement:
1. **Tool Creation**: The AI agent (AGY) analyzes architectural specifications in `docs/` and implements platform components (e.g., ticket trackers, topology advisor, inference gateway, MCP server).
2. **MCP & CLI Exposure**: Every completed capability is instantly exposed as a native CLI command and an MCP JSON-RPC tool.
3. **Recursive Self-Consumption**: In subsequent iterations, AGY and subagents call Dendrite's own MCP tools (such as querying `dendrite_plane_issues`, probing `dendrite_env_status`, checking `dendrite_inference_probe`, or running `dendrite_agent_run`) to inspect state and delegate sub-tasks.
4. **Skill Synchronization**: Completed features are fed back into this skill definition, continuously broadening the agent's autonomous capabilities across iterations.

---

## Token Economy & Delegation Matrix (Ollama Metal + AGY)

> [!IMPORTANT]
> ### Mandatory Directive: Delegate Git Commit Message Generation to Local Models
> **All autonomous agents and developer workflows MUST delegate git commit message generation to local zero-cost models (`local-qwen3-27b`, `qwen3.8:27b`, or local Ollama engine) whenever available.**
> - **Zero-Cost Mandate**: Commercial cloud models (AGY / Gemini Pro / Claude 3.5 Sonnet) must NEVER consume expensive tokens to summarize repetitive git diffs or format commit messages.
> - **Automated Execution via Dendrite**: Use `./bin/dendrite commit --ticket <ID>` or native MCP tool `dendrite_commit`. The commit engine queries the local Ollama Metal endpoint to analyze the staged diff, synthesizes a Conventional Commit message prefixed with the ticket ID, and applies the commit with auditable provenance:
>   ```bash
>   # Using Dendrite CLI
>   ./bin/dendrite commit --ticket DEN-098
>
>   # Using Dendrite Native MCP Tool
>   ./bin/dendrite mcp call dendrite_commit '{"ticket_id":"DEN-098","stage_all":true}'
>   ```
> - **Deterministic Fallback**: If local models are temporarily unavailable or offline, Dendrite automatically falls back to deterministic conventional messages without blocking workflows or burning cloud tokens.
> - **Automatic Remote Push**: dendrite commit pushes the new commit to the configured remote (origin preferred) after every successful commit; use --no-push or DENDRITE_COMMIT_PUSH=false to opt out; it never force-pushes and refuses to push main/master.


To achieve maximum engineering throughput without exhausting API token budgets or incurring runaway cloud costs, responsibilities are strictly partitioned:

| Responsibility Tier | Delegated Agent / Engine | Hardware / Runtime | Cost & Token Profile | Primary Responsibilities |
|:---|:---|:---|:---|:---|
| **Macro Architecture & Orchestration** | **AGY (Google Antigravity Cloud)** | Cloud Hosted (Gemini 2.5 Pro) | Metered Commercial API (Preserved Context) | Strategic architectural design, ticket decomposition, cross-package API contracts, security invariants, review gates. |
| **Micro Implementation & Code Loop** | **Aider Agent Loop** | Local Process / Ephemeral Container | Governed via LiteLLM Virtual Keys | TDD implementation, iterative file editing, code adjustments based on compiler errors. |
| **Commit Message Synthesis & Git Hygiene** | **Local Model (`local-qwen3-27b` / Ollama Metal)** | Apple Silicon Metal GPU / LiteLLM (:4000) | **Zero-Cost ($0.00)** Local Compute (~12ms latency) | Generating conventional commit messages, diff summarization, change classification, and provenance formatting (`[<TICKET_ID>] <type>(<scope>): <summary>`). |
| **Repetitive Execution & Verification** | **Local Ollama / Local Subagent** | Apple Silicon Metal (`qwen2.5-coder:14b`, `hermes3:8b`, `llama3.2:3b`) | **Zero-Cost ($0.00)** Local Compute (~10.9ms latency) | Running `go test`/`cargo test`, digesting raw compiler and linter output, summarizing test failures, generating boilerplate unit test vectors. |
| **Pure-Rust & Sovereign Inference** | **OxiLLaMa (`cool-japan/oxillama`)** | Local/Server Pure-Rust zero-FFI GGUF | **Zero-Cost ($0.00)** Memory-Safe | Sandboxed AST translation, sovereign offline embedding & completion. |
| **Token Budget Governance** | **Dendrite Gateway (`pkg/gateway`)** | LiteLLM Proxy (:4000) | Sliding Window Governance | Exempts self-hosted engines (`SkipBudgetChecks: true`), enforces daily/weekly ceilings, cascades to cloud APIs only on 429 quota exhaustion. |

### Model Priority & Local Intelligence Precedence Rules

1. **Smarter Local Models Take Precedence**:
   - Local models are ranked strictly by capability and parameter scale ($27\text{B}/32\text{B} > 14\text{B} > 9\text{B}/7\text{B} > 3\text{B}$).
   - **OxiLLaMa vs. Ollama Dynamic Precedence**: If Ollama is running only smaller or less-smart models (e.g. `llama3.2:3b`, `qwen2.5:1.5b`) while OxiLLaMa has a smarter model loaded (e.g. `qwen2.5-coder-7b` or `qwen3.5:9b`), **OxiLLaMa is granted strict precedence over Ollama**. Engines are never prioritized by static name; they are prioritized dynamically by the intelligence of their best loaded model.
2. **Trial Failures on Smaller Models Do NOT Count**:
   - Fast speculative attempts with smaller/lighter local models that fail are treated as zero-cost trials and are **NOT counted against the failure threshold**.
3. **Smart Failure Counting**:
   - Failures are **ONLY** counted against the failure limit when execution is attempted on the **smartest available local model(s)**.
4. **Escalation to Paid Models**:
   - Only if the smartest available local model(s) fail **twice or three times** (configurable threshold, default: 2) is the local tier declared failed. At that point, the task leverages and escalates to paid commercial models (`gemini-2.5-pro` or `copilot-sonnet`).

## Session Start & Handoff

Every new session (Claude, AGY, or human) starts by executing:
```bash
dendrite session resume
```
This restores context, displays the handoff markdown, and calculates any job status deltas since the checkpoint.

Orchestrators run:
```bash
dendrite session checkpoint [--worktree <dir>] [--decision "..."]... [--next "DEN-XXX"]... [--note "..."]
```
after each finished ticket and before long waits to persist in-flight and verified work, pending decisions, and next steps into `.dendrite/session/handoff.md`.

---

## 8-Step Lifecycle Execution Protocol

### Step 0: Identify the Target Ticket via Plane
> [!WARNING]
> **NEVER check `DEV_STATUS.md` when Plane is available.** `DEV_STATUS.md` is for humans and repo purposes only.
- Call `dendrite_plane_issues` (or run `./bin/dendrite plane issues '{"workspace":"dendrite-ws","project":"DEN"}'`) to query live issues directly from Plane.
- Select the active ticket in state `In Progress` or the first ticket in `To Do`.
- Cross-reference the ticket ID and governing specification in `docs/features/`.
- Review existing code in target packages to understand existing module layouts and interfaces.

### Step 0: Smart Ticket Selection & DAG Prioritization Protocol
- **Authoritative Selection Engine**:
  - Always execute `dendrite_ticket_smart_select` (or `./bin/dendrite plane next`) before beginning any task.
  - **Priority-First Default**: Highest-priority unblocked tickets (Urgent > High > Medium > Low) with the highest downstream critical-path impact ($W(u) = P(u) + \sum P(v)/\text{depth}$) are pulled automatically by default.
  - **User Override**: If the user explicitly specifies a ticket (e.g. `--ticket DEN-037`), the engine verifies dependency reachability, confirms all blockers are satisfied, and returns its Gantt timeline.
  - **Strict Blocker Enforcement**: Never start work on a ticket whose blockers are incomplete.
- **Plane Gantt & Relationship Mapping**:
  - When tickets, stories, or milestones are created, ensure they are placed on the development plan with non-overlapping `start_date` and `target_date`.
  - Link blocker relationships via `dendrite_plane_relation_link` (`./bin/dendrite plane link <ticket> blocked_by <blocker>`) so Plane Gantt charts visibly display dependency connection arrows.

### Step 1: Evaluate Requirement Clarity
- Audit inputs, output types, error variants, boundary behaviors, and invariants.
- Ensure acceptance criteria are testable, measurable, and deterministic.
- If requirements are underspecified, document missing constraints before writing code.

### Step 2: Evaluate Ticket Decomposition
- Determine if the ticket encapsulates a single cohesive responsibility (SRP).
- If implementation touches $> 3$ packages or exceeds ~400 lines, decompose into atomic sub-tickets directly in Plane.

### Step 3: Define Main Tests (TDD / Specification-First)
- Author unit and integration tests covering:
  1. *Happy Path*: Default behavior, valid inputs, expected outputs.
  2. *Edge Cases*: Empty inputs, max limits, special characters, timeouts.
  3. *Error Paths*: Typed errors on malformed data, network drops, or HTTP 500s.
- Target statement coverage: $\ge 85\%$ across all new modules.

### Step 4: Review Selected Design Pattern
- Verify structural patterns (e.g. Strategy pattern for engines, Factory pattern, RAII container sandboxes, sliding-window limiter).
- Enforce compile-time safety, zero-copy abstractions, and idiomatic error handling.

### Step 5: Feature Development & Implementation
- Implement types, structs, interfaces, and methods satisfying specifications.
- Wire new capabilities into CLI commands (`cmd/dendrite`) and native MCP tools (`pkg/mcpserver`).

### Step 6: Ensure Tests Pass & Static Analysis Cleans
- Run automated test suites:
  ```bash
  go test -v -cover ./pkg/<package>/...
  go test ./pkg/...
  ```
- Delegate repetitive test execution and log digestion to local agents (Ollama Metal / Aider) to conserve AGY tokens.
- All tests must pass with zero failures and statement coverage $\ge 85\%$.

### Step 7: Evaluate Code Complexity & Refactor
- Guardrails: Cyclomatic Complexity $\le 15$ per function, Cognitive Complexity $\le 20$, function length $\le 80$ lines.
- Extract helpers for nested validation chains and use early returns.

### Step 8: Plane Status, Git Commits & Skill Synchronization
- **Autonomous Git Commit via Dendrite**:
  - Execute the commit via Dendrite (`./bin/dendrite commit --ticket <ID>` or native MCP tool `dendrite_commit`), delegating commit message generation to local zero-cost models (`local-qwen3-27b` / Ollama Metal) to preserve token budget.
  - `dendrite commit` pushes the new commit to the configured remote (origin preferred) after every successful commit; use `--no-push` or `DENDRITE_COMMIT_PUSH=false` to opt out; it never force-pushes and refuses to push main/master.
- Mutate the ticket state directly in Plane:
  - Transition the issue state to `To Verify` in Plane via the Plane API / client or `./bin/dendrite chronos ticket set <ID> "to verify"`.
  - Open its GitHub verification issue with `./bin/dendrite myelin github-sync --ticket <ID>` (preview with `--dry-run`).
  - Autonomous agents must NEVER mark an issue directly as `Done`; issues remain in `To Verify` until formally signed off through human verification (`dendrite allostasis hitl-sync`, `dendrite myelin github-sync --hitl` or the Plane web UI).
  - Advance next ticket to `In Progress` in Plane.
- Synchronize repository documentation for human readers:
  - Run `dendrite plane replicate` (or native MCP `dendrite_plane_replicate`) so that `DEV_STATUS.md` is updated from Plane for human repo viewers.
- **Update Skills**: Add newly completed tools, CLI commands, and MCP tools directly to this `SKILL.md` across `.gemini/skills/` and `.agents/skills/`.