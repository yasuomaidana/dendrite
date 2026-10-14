# Autonomous Coding Agent Harness & Tool Orchestration (Aider, OpenHands, OpenCode, Google AGY)

# FEAT-013: Autonomous Coding Agent Harness & Tool Orchestration (Aider, OpenHands, OpenCode, Google AGY)

## 1. User Story & Business Objective

As a software engineer and platform architect, I need Dendrite to natively support and orchestrate premier autonomous coding agents—specifically **Aider**, **OpenHands** (formerly OpenDevin), **OpenCode**, and **Google AGY (Antigravity)**—by automatically bootstrapping their configurations, routing their API traffic through the LiteLLM Token Governance Gateway, enforcing sliding window budget caps and thermal curfews, injecting AST-compacted repository context, and executing them within ephemeral, cap-dropped container sandboxes with automated issue tracker status synchronization.

---

## 2. Technical Boundaries & Specifications

1. **Unified Agent Adapter Interface & Tool Registry (`pkg/agent`)**:

- Provides a typed abstraction (`AgentRunner`) supporting multiple coding agent engines:
- **Aider (`aider`)**: Terminal-centric CLI pair programming agent with tight Git repository integration and commit generation.
- **OpenHands (`openhands`)**: Containerized autonomous software engineer capable of running arbitrary bash commands, editing code, and browsing documentation.
- **OpenCode (`opencode`)**: Headless, lightweight automated code generation, refactoring, and batch pull-request agent.
- **Google AGY (`agy`, `antigravity`)**: Google DeepMind autonomous agent CLI supporting multi-directory workspaces (`--add-dir`), non-interactive task prompts (`-p`), and automated permission approval (`--dangerously-skip-permissions`).

2. **Automated Gateway Binding & Virtual Key Injection**:

- Automatically generates ephemeral or workspace-scoped LiteLLM Virtual Keys for the agent session with strict budget ceilings (e.g. $5.00/session or $20.00/week).
- Injects standard OpenAI/Anthropic/Gemini environment variables into the agent runtime:
- `OPENAI_API_BASE=http://<control-plane-host>:4000/v1`
- `OPENAI_API_KEY=<dendrite-virtual-key>`
- `GOOGLE_GEMINI_BASE_URL=http://<control-plane-host>:4000/v1`
- `GEMINI_API_KEY=<dendrite-virtual-key>`
- Model tier overrides (e.g., directing primary edits to `copilot-sonnet` or `gemini-2.5-pro` and automated test generation to `local-qwen-coder`).

3. **AST Context Injection & Semantic Prompt Compaction**:

- Prior to agent startup, extracts AST symbol graphs via `pkg/ast` and `pkg/devtool`, generating a compacted repository context envelope.
- For Aider: Formats context as a repository map (`--map-tokens 2048` or custom repo prompt).
- For OpenHands / OpenCode / AGY: Pre-loads the workspace context envelope into the initial prompt instructions.

4. **Sandboxed & Ephemeral Execution Modes**:

- **Host Mode**: Runs directly in the developer's local terminal workspace (primarily for interactive Aider or AGY sessions).
- **Ephemeral Sandbox Mode**: Spawns agent inside a cap-dropped, read-only root Docker container (`pkg/sandbox`) with an isolated scratch volume and automatic RAII teardown upon process exit (`google/antigravity:latest`).

5. **Issue Tracker & Git Provenance Linking**:

- Links agent sessions to active tickets (e.g., `DEN-006`, `DEN-028` or Jira/Plane issue keys).
- Formats git commit messages automatically: `[<TICKET-ID>] <Action Summary> (Agent: <aider|openhands|opencode|agy>)`.
- Posts session summaries and token spend metrics back to the active issue tracker.

flowchart TD
User([Developer / CI Trigger]) --&gt;|dendrite agent run &lt;tool&gt;| CLI[Dendrite CLI]

subgraph &#34;Dendrite Control Plane &amp; Orchestrator&#34;

CLI --&gt; AgentEngine[Agent Orchestration Engine&lt;br/&gt;pkg/agent]

AgentEngine --&gt; TokenGov[LiteLLM Gateway Virtual Key Injector&lt;br/&gt;pkg/gateway]

AgentEngine --&gt; ContextGen[AST Context Envelope Injector&lt;br/&gt;pkg/ast]

AgentEngine --&gt; TrackerSync[Issue Tracker Sync&lt;br/&gt;pkg/tracker]

end

subgraph &#34;Execution Target&#34;

AgentEngine --&gt;|Local Terminal| HostExec[Host Workspace]

AgentEngine --&gt;|Ephemeral Flag| SandboxExec[Sandboxed Container&lt;br/&gt;pkg/sandbox]

end

subgraph &#34;Agent Engines&#34;

HostExec &amp; SandboxExec --&gt; Aider[&#34;Aider&lt;br/&gt;(Git Pair Programmer)&#34;]

HostExec &amp; SandboxExec --&gt; OpenHands[&#34;OpenHands&lt;br/&gt;(Full Autonomous Agent)&#34;]

HostExec &amp; SandboxExec --&gt; OpenCode[&#34;OpenCode&lt;br/&gt;(Headless Batch Refactoring)&#34;]

HostExec &amp; SandboxExec --&gt; AGY[&#34;Google AGY&lt;br/&gt;(Antigravity CLI Agent)&#34;]

end

Aider &amp; OpenHands &amp; OpenCode &amp; AGY --&gt;|Governed API Calls| Gateway[LiteLLM Gateway :4000]

Gateway --&gt; Fallback[Cascading Fallback &amp; Spend Guardrails]</code></pre>

---

## 3. Subcommand Specifications & Ergonomics
# 1. Run interactive Aider session bound to Dendrite's LiteLLM gateway and current ticket
dendrite agent run aider --ticket DEN-006 --model copilot-sonnet

# 2. Run OpenHands in an ephemeral sandbox to resolve an issue

dendrite agent run openhands --ticket PROJ-104 --sandbox --spend-limit 5.00

# 3. Run OpenCode in headless batch mode for automated refactoring

dendrite agent run opencode --task &#34;Refactor pkg/ast to reduce cognitive complexity&#34; --ephemeral

# 4. Run Google AGY (Antigravity) bound to LiteLLM gateway and ticket

dendrite agent run agy --ticket DEN-028 --task &#34;Implement AST compaction harness&#34; --model gemini-2.5-pro

# 5. Display active and past agent sessions, spend metrics, and generated commits

dendrite agent session list</code></pre>

---

## 4. Acceptance Criteria & Verification Matrix

- [x] **AC-1**: `dendrite agent run <aider|openhands|opencode|agy>` successfully configures the runtime environment with LiteLLM proxy base URL, virtual API keys, and model mappings without hardcoded secrets.
- [x] **AC-2**: When budget limits or thermal curfews are reached, the agent harness cleanly terminates the process, commits progress, and posts spend logs to the active issue tracker.
- [x] **AC-3**: Sandboxed execution (`--sandbox` / `--ephemeral`) boots the agent in an isolated container with zero unauthorized host filesystem mutations.
- [x] **AC-4**: Commits produced during agent sessions include structured ticket metadata and agent provenance tags.

---

## 5. Definition of Done (DoD) & Persistence Verification

- **Automated Tests**: Unit tests assert environment variable generation, argument rendering, and config validation across all supported agent types with $\ge 85\%$ statement coverage.
- **Persistent Layer Verification**: Session history and spend records persisted in SQLite/PostgreSQL verified via real container instances using [Testcontainers](https://rust.testcontainers.org).
- **Complexity Guardrails**: Orchestrator functions adhere to Cyclomatic Complexity $\le 10$, Cognitive Complexity $\le 12$, and function length $\le 60$ LOC.
