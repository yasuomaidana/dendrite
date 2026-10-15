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
  2. `dendrite_plane_replicate`: Replicate `DEV_STATUS.md` directly from Plane service state (Plane owns project status; dev_status is for humans and repo purposes only).
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
  11. `dendrite_agent_run`: Launch an autonomous coding agent loop (Aider, OpenHands, OpenCode, AGY) with LiteLLM token governance, commit provenance, and sandboxing.
      - *Example*: `./bin/dendrite mcp call dendrite_agent_run '{\"agent_type\":\"aider\",\"task\":\"Implement scenario runner\",\"ticket_id\":\"DEN-037\"}'`
  12. `dendrite_topology_advisor`: Evaluate hardware capabilities of compute nodes and calculate optimal placement of Dendrite microservices.
      - *Example*: `./bin/dendrite mcp call dendrite_topology_advisor '{"nodes":"local=localhost"}'`
  13. `dendrite_gateway_render`: Render dynamic LiteLLM YAML routing configuration for a given topology scenario (`local-hybrid`, `cloud-burst`, `full-local`).
      - *Example*: `./bin/dendrite mcp call dendrite_gateway_render '{"scenario":"local-hybrid"}'`
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

### 2. Dendrite CLI Tools (`./bin/dendrite`)
- `dendrite agent <search|route>`: Search and rank agents with hybrid vector/BM25 search or auto-route tasks to optimal agents with LiteLLM virtual keys.
- `dendrite test <auto|ui>`: Synthesize table-driven tests with Testcontainers & self-healing convergence, or run Web UI browser journeys & WebSocket telemetry load tests.
- `dendrite triage [--prompt "..."] [--file <file>] [--tier local|hybrid|cloud] [--local]`: Front-door local agent prompt router, skill injector, and token efficiency optimizer.
- `dendrite mcp <serve|tools|call>`: Native Model Context Protocol stdio server and testing harness.
- `dendrite jetbrains <probe|open|problems|symbols|build|run|setup>`: Probe JetBrains MCP server, trigger IDE builds/runs, and configure IDE toolsets.
- `dendrite agent run <aider|openhands|opencode|agy> [flags]`: Launch autonomous agent sessions bound to LiteLLM virtual keys and ephemeral containers.
- `dendrite inference <list|probe|switch|render>`: Inspect and manage multi-provider inference backends (Ollama Metal, OxiLLaMa, TGI, TEI, vLLM, llama.cpp).
- `dendrite dev inference <list|probe|switch|render>`: Development inference gateway control and route rendering.
- `dendrite plane replicate [--file DEV_STATUS.md]`: Replicate `DEV_STATUS.md` from Plane service (Plane owns project status; dev_status is for humans and repo purposes only).
- `dendrite plane push-status`: Move local markdown content (tickets, milestones, specs) to Plane service.
- `dendrite plane doc <push|pull|sync> [--dir docs]`: Bidirectional documentation synchronization between Plane and local repository.
- `dendrite plane <issues|modules|pages>`: Query live Plane issues, milestones/modules, and documentation pages.
- `dendrite dev ticket <list|set|push-plane|replicate-plane>`: Inspect or mutate ticket status and sync with Plane.
- `dendrite dev gateway [scenario]`: Render dynamic LiteLLM configuration YAML.
- `dendrite topology advisor [--nodes ...]`: Run remote SSH/local hardware discovery and dynamic microservice placement routing.
- `dendrite telemetry [metrics|status]`: Emit Prometheus exposition metrics or real-time Apple Silicon SoC thermals.
- `dendrite env <doctor|plan|install|replicate|status>`: Target computer diagnostic and container replication engine.
- `dendrite dist <sources|resolve|decompose|interop|spawn>`: Manage distributed codebase sources, modular REA, cross-project interop, and parallel project/Plane spawner.
- `dendrite bench <inference|gateway|matrix|stress>`: Benchmark inference TTFT/TPS velocity, stress-test LiteLLM gateway failover cascades, run multi-engine matrix, or stress budget limiter.
- `dendrite rea <showcase|list> [target] [--format text|json|markdown]`: Open-source REA cleanroom analysis, evidence extraction, and property test verification suite across 5 target archetypes.\n\n- `dendrite pool <add|remove|list|resolve> [--json]`: Autonomous multi-directory project pool manager and deterministic topological DAG solver with cycle detection.
- `dendrite query [slice] --sources <path> --entrypoint <symbol> [flags] "<intent>"`: Universal intent-driven CFG/AST reachability slicer with golden vector test generation.
- `dendrite query transpile --sources <artifact> [--to rust] [--no-std] [flags] "<prompt>"`: Safe idiomatic Rust transpiler with differential parity harness.
- `dendrite query fuse --sources <src1,src2> --name <member> [--target-dir <dir>]`: Multi-artifact architecture fusion and in-process channel synthesizer.

### 3. Unified Stacks Environment (`stacks/.env`)\nDendrite maintains an enterprise-grade multi-service container orchestration stack parameterized via `stacks/.env` (and mirrored in `.env`):\n- **Core Control Plane**: PostgreSQL 16 (`5432:5432`) & Redis 7 (`6379:6379`)\n- **Token Governance Gateway**: LiteLLM Proxy (`4000:4000`) with master key authentication, budget ceilings, and route cascades (`stacks/litellm_config.yaml`).\n- **Self-Hosted Project Management**: Plane v2 Backend (`8090`), Web UI (`8091`), PostgreSQL 16 (`5434`), Redis 7 (`6380`), and Nginx Proxy.\n- **Local Issue Tracker Mock**: Linear Mock Server (`8085`) and dedicated PostgreSQL 16 (`5433`).\n- **Design & Prototyping Platform**: Penpot Frontend (`9000`), Backend (`9001`), PostgreSQL 16 (`5435`), and Redis 7 (`6381`).\n- **Observability Suite**: Prometheus 2.50 (`9090`) and Grafana 10.3 (`3000`) with automated dashboards and data sources.\n- **Pluggable Inference Upstreams**: Local Ollama (`11434`), OxiLLaMa (`8080`), Hugging Face TGI (`8080`), TEI (`8082`), vLLM (`8000`), and llama.cpp (`8081`).\n\n### 4. Continuous Skill Enhancement Mandate\n> [!IMPORTANT]\n> **As soon as you finish a tool or feature, add it to the skills!**\n> Whenever a new CLI subcommand, MCP tool, or runtime adapter is implemented, immediately update `SKILL.md` across `.gemini/skills/` and `.agents/skills/` so subsequent agents and turns can leverage it immediately.\n\n---\n\n
### 5. Documentation Cleanliness & Taxonomy Mandate
> [!IMPORTANT]
> **Keep Documentation Clean, Structured, and Taxonomically Sound!**
> - Feature RFCs and specifications must strictly reside in `docs/features/FEAT-XXX-*.md`.
> - Operational workflows and end-to-end scenarios must reside in `docs/SCENARIOS.md` or `docs/use-cases/`.
> - Never pollute the project root or top-level `docs/` with uncurated notes or loose scratch files.
> - Ensure cross-reference links between specifications, CLI help, and skills are consistently maintained.

## Autonomous Agentic Loop: Aider Integration & Delegation\n\nDendrite standardizes on **Aider** as the primary autonomous agentic coding loop (with optional support for OpenHands and OpenCode):\n- **Execution Harness**: Executed via CLI:\n  ```bash\n  ./bin/dendrite agent run aider --ticket <ID> --task \"<task>\" --model copilot-sonnet\n  ```\n  or via native MCP tool:\n  ```bash\n  ./bin/dendrite mcp call dendrite_agent_run '{\"agent_type\":\"aider\",\"task\":\"...\",\"ticket_id\":\"<ID>\"}'\n  ```\n- **LiteLLM Virtual Key Governance**: Aider communicates through Dendrite's LiteLLM proxy gateway (`http://localhost:4000/v1`) with zero-cost financial exemptions for self-hosted engines.\n- **Git Commit Provenance**: All commits generated by the agentic loop are automatically tagged with ticket provenance (e.g. `[DEN-037] Automated change via agent (Agent: aider)`).\n- **Ephemeral Sandbox Isolation**: Option `--sandbox` executes Aider inside hardened Docker containers with read-only root mounts and temporary RAM-backed scratch dirs.\n\n---\n\n## Iterative Self-Improvement Showcase\n\nThis project is a living showcase of recursive agentic self-improvement:\n1. **Tool Creation**: The AI agent (AGY) analyzes architectural specifications in `docs/` and implements platform components (e.g., ticket trackers, topology advisor, inference gateway, MCP server).\n2. **MCP & CLI Exposure**: Every completed capability is instantly exposed as a native CLI command and an MCP JSON-RPC tool.\n3. **Recursive Self-Consumption**: In subsequent iterations, AGY and subagents call Dendrite's own MCP tools (such as querying `dendrite_plane_issues`, probing `dendrite_env_status`, checking `dendrite_inference_probe`, or running `dendrite_agent_run`) to inspect state and delegate sub-tasks.\n4. **Skill Synchronization**: Completed features are fed back into this skill definition, continuously broadening the agent's autonomous capabilities across iterations.\n\n---\n\n## Token Economy & Delegation Matrix (Ollama Metal + AGY)\n\nTo achieve maximum engineering throughput without exhausting API token budgets or incurring runaway cloud costs, responsibilities are strictly partitioned:\n\n| Responsibility Tier | Delegated Agent / Engine | Hardware / Runtime | Cost & Token Profile | Primary Responsibilities |\n|:---|:---|:---|:---|:---|\n| **Macro Architecture & Orchestration** | **AGY (Google Antigravity Cloud)** | Cloud Hosted (Gemini 2.5 Pro) | Metered Commercial API (Preserved Context) | Strategic architectural design, ticket decomposition, cross-package API contracts, security invariants, review gates. |\n| **Micro Implementation & Code Loop** | **Aider Agent Loop** | Local Process / Ephemeral Container | Governed via LiteLLM Virtual Keys | TDD implementation, iterative file editing, code adjustments based on compiler errors. |\n| **Repetitive Execution & Verification** | **Local Ollama / Local Subagent** | Apple Silicon Metal (`qwen2.5-coder:14b`, `hermes3:8b`, `llama3.2:3b`) | **Zero-Cost ($0.00)** Local Compute (~10.9ms latency) | Running `go test`/`cargo test`, digesting raw compiler and linter output, summarizing test failures, generating boilerplate unit test vectors. |\n| **Pure-Rust & Sovereign Inference** | **OxiLLaMa (`cool-japan/oxillama`)** | Local/Server Pure-Rust zero-FFI GGUF | **Zero-Cost ($0.00)** Memory-Safe | Sandboxed AST translation, sovereign offline embedding & completion. |\n| **Token Budget Governance** | **Dendrite Gateway (`pkg/gateway`)** | LiteLLM Proxy (:4000) | Sliding Window Governance | Exempts self-hosted engines (`SkipBudgetChecks: true`), enforces daily/weekly ceilings, cascades to cloud APIs only on 429 quota exhaustion. |\n\n---\n\n## 8-Step Lifecycle Execution Protocol\n\n### Step 0: Identify the Target Ticket via Plane\n> [!WARNING]\n> **NEVER check `DEV_STATUS.md` when Plane is available.** `DEV_STATUS.md` is for humans and repo purposes only.\n- Call `dendrite_plane_issues` (or run `./bin/dendrite plane issues '{\"workspace\":\"dendrite-ws\",\"project\":\"DEN\"}'`) to query live issues directly from Plane.\n- Select the active ticket in state `In Progress` or the first ticket in `To Do`.\n- Cross-reference the ticket ID and governing specification in `docs/features/`.\n- Review existing code in target packages to understand existing module layouts and interfaces.\n\n### Step 1: Evaluate Requirement Clarity\n- Audit inputs, output types, error variants, boundary behaviors, and invariants.\n- Ensure acceptance criteria are testable, measurable, and deterministic.\n- If requirements are underspecified, document missing constraints before writing code.\n\n### Step 2: Evaluate Ticket Decomposition\n- Determine if the ticket encapsulates a single cohesive responsibility (SRP).\n- If implementation touches $> 3$ packages or exceeds ~400 lines, decompose into atomic sub-tickets directly in Plane.\n\n### Step 3: Define Main Tests (TDD / Specification-First)\n- Author unit and integration tests covering:\n  1. *Happy Path*: Default behavior, valid inputs, expected outputs.\n  2. *Edge Cases*: Empty inputs, max limits, special characters, timeouts.\n  3. *Error Paths*: Typed errors on malformed data, network drops, or HTTP 500s.\n- Target statement coverage: $\\ge 85\\%$ across all new modules.\n\n### Step 4: Review Selected Design Pattern\n- Verify structural patterns (e.g. Strategy pattern for engines, Factory pattern, RAII container sandboxes, sliding-window limiter).\n- Enforce compile-time safety, zero-copy abstractions, and idiomatic error handling.\n\n### Step 5: Feature Development & Implementation\n- Implement types, structs, interfaces, and methods satisfying specifications.\n- Wire new capabilities into CLI commands (`cmd/dendrite`) and native MCP tools (`pkg/mcpserver`).\n\n### Step 6: Ensure Tests Pass & Static Analysis Cleans\n- Run automated test suites:\n  ```bash\n  go test -v -cover ./pkg/<package>/...\n  go test ./pkg/...\n  ```\n- Delegate repetitive test execution and log digestion to local agents (Ollama Metal / Aider) to conserve AGY tokens.\n- All tests must pass with zero failures and statement coverage $\\ge 85\\%$.\n\n### Step 7: Evaluate Code Complexity & Refactor\n- Guardrails: Cyclomatic Complexity $\\le 15$ per function, Cognitive Complexity $\\le 20$, function length $\\le 80$ lines.\n- Extract helpers for nested validation chains and use early returns.\n\n### Step 8: Plane Status & Skill Synchronization\n- Mutate the ticket state directly in Plane:\n  - Update the issue state to `Done` in Plane via the Plane API / client or `./bin/dendrite dev ticket set <ID> DONE --push-plane`.\n  - Advance next ticket to `In Progress` in Plane.\n- Synchronize repository documentation for human readers:\n  - Run `dendrite plane replicate` (or native MCP `dendrite_plane_replicate`) so that `DEV_STATUS.md` is updated from Plane for human repo viewers.\n- **Update Skills**: Add newly completed tools, CLI commands, and MCP tools directly to this `SKILL.md` across `.gemini/skills/` and `.agents/skills/`.\n