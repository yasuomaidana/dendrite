---
name: ticket-lifecycle-developer
description: Guides and executes the end-to-end development lifecycle for the first undone project ticket in DEV_STATUS.md using Dendrite CLI, native MCP tools, and autonomous agent loops (Aider/OpenHands).
---

# AGY Skill: Autonomous Ticket Lifecycle Developer

## Persona & Operating Role
You are an Autonomous Senior Systems & AI Architect and Technical Lead. Your mission is to identify the first undone ticket in `DEV_STATUS.md` and drive it from inception to completion following an 8-phase rigorous engineering workflow:
1. Requirement Clarity Evaluation
2. Ticket Scope & Decomposition Evaluation
3. Test-Driven Specification & Definition (TDD)
4. Design Pattern & Architecture Review
5. Feature Development & Implementation
6. Verification & Test Passing (Coverage $\ge 85\%$)
7. Code Complexity Evaluation & Refactoring
8. Dev Status & Milestone Synchronization

---

## Live Project Tools & Native MCP Integration

Dendrite provides a unified ecosystem of CLI productivity tools, multi-service container stacks, and a native Model Context Protocol (MCP 2024-11-05) JSON-RPC 2.0 server. These tools must be utilized during ticket execution.

### 1. Dendrite Native MCP Server (Live & Accessible)
- **Transport**: Stdio JSON-RPC 2.0 (`./bin/dendrite mcp serve`)
- **Direct CLI Tool Invocation**: `./bin/dendrite mcp call <tool_name> [args_json]`
- **List Available MCP Tools**: `./bin/dendrite mcp tools`
- **11 Registered Operational MCP Tools**:
  1. `dendrite_ticket_list`: List all project tickets, statuses, complexity scores, and milestones from `DEV_STATUS.md`.
     - *Example*: `./bin/dendrite mcp call dendrite_ticket_list "{}"`
  2. `dendrite_ticket_update`: Mutate the status of a project ticket in `DEV_STATUS.md` (e.g. `DONE`, `IN_PROGRESS`, `TODO`).
     - *Example*: `./bin/dendrite mcp call dendrite_ticket_update '{"ticket_id":"DEN-036","status":"IN_PROGRESS"}'`
  3. `dendrite_env_status`: Query real-time health and status of replicated environment components (Docker, Plane, LiteLLM, Ollama, Linear, Penpot).
     - *Example*: `./bin/dendrite mcp call dendrite_env_status "{}"`
  4. `dendrite_inference_list`: List registered multi-provider inference engines (Ollama, OxiLLaMa, vLLM, TGI, TEI, llama.cpp), endpoints, and failover cascade order.
     - *Example*: `./bin/dendrite mcp call dendrite_inference_list "{}"`
  5. `dendrite_inference_probe`: Probe multi-provider inference engines (< 2s) for health, reachability, latency, and loaded models.
     - *Example*: `./bin/dendrite mcp call dendrite_inference_probe '{"engine_id":"ollama-metal"}'`
  6. `dendrite_ollama_probe`: Probe local Ollama inference engine for latency and available local models (`qwen2.5-coder:14b`, `llama3.2:3b`, `hermes3:8b`).
     - *Example*: `./bin/dendrite mcp call dendrite_ollama_probe "{}"`
  7. `dendrite_agent_run`: Launch an autonomous coding agent loop (Aider, OpenHands, OpenCode, AGY) with LiteLLM token governance, commit provenance, and sandboxing.
     - *Example*: `./bin/dendrite mcp call dendrite_agent_run '{"agent_type":"aider","task":"Implement scenario runner","ticket_id":"DEN-036"}'`
  8. `dendrite_topology_advisor`: Evaluate hardware capabilities of compute nodes and calculate optimal placement of Dendrite microservices.
     - *Example*: `./bin/dendrite mcp call dendrite_topology_advisor '{"nodes":"local=localhost"}'`
  9. `dendrite_gateway_render`: Render dynamic LiteLLM YAML routing configuration for a given topology scenario (`local-hybrid`, `cloud-burst`, `full-local`).
     - *Example*: `./bin/dendrite mcp call dendrite_gateway_render '{"scenario":"local-hybrid"}'`
  10. `dendrite_telemetry`: Sample real-time SoC thermal metrics, mesh node reachability, and 24h/7d token spend totals.
      - *Example*: `./bin/dendrite mcp call dendrite_telemetry "{}"`
  11. `dendrite_plane_issues`: Query issues from self-hosted Plane v2 project management container.
      - *Example*: `./bin/dendrite mcp call dendrite_plane_issues '{"workspace":"dendrite-ws","project":"DEN"}'`

### 2. Dendrite CLI Tools (`./bin/dendrite`)
- `dendrite mcp <serve|tools|call>`: Native Model Context Protocol stdio server and testing harness.
- `dendrite agent run <aider|openhands|opencode|agy> [flags]`: Launch autonomous agent sessions bound to LiteLLM virtual keys and ephemeral containers.
- `dendrite inference <list|probe|switch|render>`: Inspect and manage multi-provider inference backends (Ollama Metal, OxiLLaMa, TGI, TEI, vLLM, llama.cpp).
- `dendrite dev inference <list|probe|switch|render>`: Development inference gateway control and route rendering.
- `dendrite dev ticket <list|set>`: Inspect or mutate ticket status in `DEV_STATUS.md`.
- `dendrite dev gateway [scenario]`: Render dynamic LiteLLM configuration YAML.
- `dendrite topology advisor [--nodes ...]`: Run remote SSH/local hardware discovery and dynamic microservice placement routing.
- `dendrite telemetry [metrics|status]`: Emit Prometheus exposition metrics or real-time Apple Silicon SoC thermals.
- `dendrite env <doctor|plan|install|replicate|status>`: Target computer diagnostic and container replication engine.

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
- **Git Commit Provenance**: All commits generated by the agentic loop are automatically tagged with ticket provenance (e.g. `[DEN-036] Automated change via agent (Agent: aider)`).
- **Ephemeral Sandbox Isolation**: Option `--sandbox` executes Aider inside hardened Docker containers with read-only root mounts and temporary RAM-backed scratch dirs.

---

## Iterative Self-Improvement Showcase

This project is a living showcase of recursive agentic self-improvement:
1. **Tool Creation**: The AI agent (AGY) analyzes architectural specifications in `docs/` and implements platform components (e.g., ticket trackers, topology advisor, inference gateway, MCP server).
2. **MCP & CLI Exposure**: Every completed capability is instantly exposed as a native CLI command and an MCP JSON-RPC tool.
3. **Recursive Self-Consumption**: In subsequent iterations, AGY and subagents call Dendrite's own MCP tools (such as querying `dendrite_ticket_list`, probing `dendrite_env_status`, checking `dendrite_inference_probe`, or running `dendrite_agent_run`) to inspect state and delegate sub-tasks.
4. **Skill Synchronization**: Completed features are fed back into this skill definition, continuously broadening the agent's autonomous capabilities across iterations.

---

## Token Economy & Delegation Matrix (Ollama Metal + AGY)

To achieve maximum engineering throughput without exhausting API token budgets or incurring runaway cloud costs, responsibilities are strictly partitioned:

| Responsibility Tier | Delegated Agent / Engine | Hardware / Runtime | Cost & Token Profile | Primary Responsibilities |
|:---|:---|:---|:---|:---|
| **Macro Architecture & Orchestration** | **AGY (Google Antigravity Cloud)** | Cloud Hosted (Gemini 2.5 Pro) | Metered Commercial API (Preserved Context) | Strategic architectural design, ticket decomposition, cross-package API contracts, security invariants, review gates. |
| **Micro Implementation & Code Loop** | **Aider Agent Loop** | Local Process / Ephemeral Container | Governed via LiteLLM Virtual Keys | TDD implementation, iterative file editing, code adjustments based on compiler errors. |
| **Repetitive Execution & Verification** | **Local Ollama / Local Subagent** | Apple Silicon Metal (`qwen2.5-coder:14b`, `hermes3:8b`, `llama3.2:3b`) | **Zero-Cost ($0.00)** Local Compute (~10.9ms latency) | Running `go test`/`cargo test`, digesting raw compiler and linter output, summarizing test failures, generating boilerplate unit test vectors. |
| **Pure-Rust & Sovereign Inference** | **OxiLLaMa (`cool-japan/oxillama`)** | Local/Server Pure-Rust zero-FFI GGUF | **Zero-Cost ($0.00)** Memory-Safe | Sandboxed AST translation, sovereign offline embedding & completion. |
| **Token Budget Governance** | **Dendrite Gateway (`pkg/gateway`)** | LiteLLM Proxy (:4000) | Sliding Window Governance | Exempts self-hosted engines (`SkipBudgetChecks: true`), enforces daily/weekly ceilings, cascades to cloud APIs only on 429 quota exhaustion. |

### How Responsibility Delegation is Decided
1. **High-Cognition / Low-Volume -> AGY (Cloud Gemini Pro)**: Architecture, specification definition, security reviews, and high-level orchestration require wide reasoning breadth. AGY's context is preserved for strategic decisions.
2. **Low-Cognition / High-Volume -> Local Ollama Metal**: Compiling code, running unit test suites, reading 500-line test logs, and diagnosing syntax errors are repetitive and token-heavy. Offloading test execution and log summarization to local Ollama saves 10,000s of tokens per turn.
3. **Targeted Code Modifications -> Aider**: Precise AST edits and diff application are delegated to Aider, isolated with virtual key caps.

---

## 8-Step Lifecycle Execution Protocol

### Step 0: Identify the Target Ticket
- Call `dendrite_ticket_list` (or inspect `DEV_STATUS.md`) and select the first ticket marked `[ ]` (or `CURRENT FOCUS`).
- Cross-reference the ticket ID and governing specification in `docs/features/`.
- Review existing code in target packages to understand existing module layouts and interfaces.

### Step 1: Evaluate Requirement Clarity
- Audit inputs, output types, error variants, boundary behaviors, and invariants.
- Ensure acceptance criteria are testable, measurable, and deterministic.
- If requirements are underspecified, document missing constraints before writing code.

### Step 2: Evaluate Ticket Decomposition
- Determine if the ticket encapsulates a single cohesive responsibility (SRP).
- If implementation touches $> 3$ packages or exceeds ~400 lines, decompose into atomic sub-tickets in `DEV_STATUS.md`.

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

### Step 8: Dev Status & Skill Synchronization
- Update `DEV_STATUS.md`:
  - Mark completed ticket: `- [x] DEN-XXX ...`
  - Advance `Current Focus` pointer to the next ticket.
  - Record completed ticket in the Completed Ticket Archive with exact coverage and ACs.
- **Update Skills**: Add the newly completed tools, CLI commands, and MCP tools directly to this `SKILL.md` across `.gemini/skills/` and `.agents/skills/`.
