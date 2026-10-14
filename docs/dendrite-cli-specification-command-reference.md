# Dendrite CLI Specification & Command Reference

# Dendrite CLI Specification & Command Reference

# Dendrite CLI Specification & Command Reference

The `dendrite` CLI is the unified control plane binary for managing multi-node deployment scenarios, token governance, multi-project workspaces, issue tracker providers (Jira, Plane, Linear), autonomous coding agent harnesses (Aider, OpenHands, OpenCode, Google AGY), AI-assisted code optimization, polyglot translation, documentation generation/reflection, and ephemeral sandboxed analysis.

---

## 1. CLI Architecture & Design Patterns

Implemented in idiomatic Go (with GoLand native ergonomics) or Rust, the CLI adheres to strict architectural patterns:

1. **Typestate Lifecycle Transitions**:

- Commands enforce valid state transitions at compile-time/runtime:

`Workspace<Unconfigured>` $\xrightarrow{\text{init}}$ `Workspace<Configured>` $\xrightarrow{\text{switch}}$ `Workspace<Active>`.

2. **RAII & Resource Cleanup Semantics**:

- Ephemeral commands use deferred teardown traps (`defer c.Cleanup()` or `Drop`) to guarantee container removal, network detachment, and tmpfs unmounting even during runtime panics or `SIGINT` (Ctrl+C).

3. **Strategy Pattern for Pluggable Providers**:

- Issue tracker implementations (Jira, Plane, Linear, GitHub Issues) and coding agent runners (Aider, OpenHands, OpenCode, Google AGY) adhere to decoupled interfaces (`IssueTracker`, `AgentRunner`), allowing frictionless provider switching via CLI flags or config.

4. **Intermediate Semantic Representation (ISR)**:

- Polyglot translation and capability import decouple source syntax from target synthesis via typed Tree-sitter AST nodes.

5. **Domain Newtypes & Type Safety**:

- Wrapping primitive IDs into dedicated types (`type ProjectID string`, `type VirtualKey string`, `type MeshNode string`, `type IssueID string`, `type AgentType string`).

6. **Complexity Guardrails**:

- Every CLI subcommand method is bounded: Cyclomatic Complexity $\le 10$, Cognitive Complexity $\le 15$, LOC per function $\le 60$.

---

## 2. Command Hierarchy & Syntax Matrix

```
dendrite [global flags]  [args...]
```

```
├── doctor                           # Pre-flight environment diagnostics
├── env                              # Development Environment Installation & Replication

│   ├── doctor                       # Inspect host OS, package manager & installed tools

│   ├── plan [flags]                 # Plan replication (--mode container|host|hybrid)

│   ├── replicate [flags]            # Replicate dev environment (containers or host)

│   ├── install <tool> [flags]       # Install tool on host or start container (e.g. Linear)

│   └── status                       # Real-time health & status of replicated components

├── init                             # Guided bootstrap and scenario setup

├── up [scenario]                    # Apply topology & launch active profile stacks

│                                    # (local-hybrid | azure-burst | aws-burst | distributed)

├── down                             # Gracefully halt active stacks

├── status                           # System health, spend dashboard & thermal monitor

├── ui [flags]                       # Launch Dendrite Web UI & Configuration Dashboard

│

├── agent                            # Autonomous Coding Agent Orchestration

│   ├── run <aider|openhands|opencode|agy> # Launch coding agent with LiteLLM gateway & token governance

│   ├── config <agent> [flags]       # Configure agent model tiers, budget limits, sandbox mode

│   └── session                      # Inspect active/past agent sessions, spend & commits

│

├── project                          # Multi-Project Workspace Management

│   ├── add <name> <path-or-git>     # Register a project into Dendrite registry

│   ├── list                         # List registered projects and active marker

│   ├── switch <name>                # Switch active project context & MCP database

│   ├── status                       # Display active project statistics & spend

│   └── remove <name>                # Unregister project and prune local cache

│

├── tracker                          # Pluggable Issue Tracker Management

│   ├── list                         # List available and active issue tracker providers

│   ├── switch <jira|plane|linear>   # Switch active issue tracker for current workspace

│   ├── config <provider> [flags]    # Configure API URLs, tokens, and project keys

│   └── test                         # Verify API connectivity and issue permissions

│

├── ai                               # AI-Assisted Development & Optimization

│   ├── optimize-tokens [flags]      # Semantic prompt compaction & prefix cache setup

│   └── import [flags]               # Dissect external repo & cleanroom import functionality

│

├── translate [flags]                # Polyglot code translation with dual test parity

│

├── doc                              # Docs-as-Code Generation & Reflection

│   ├── gen [flags]                  # Synthesize FEAT-*, ADR-*, and API specs from AST

│   └── reflect [flags]              # Mirror reference project docs hierarchy into target

│

├── jetbrains                        # JetBrains IDE MCP Integration & Bridge

│   ├── probe [url]                  # Discover IDE MCP capabilities & tools

│   ├── open <file> [--line <n>]     # Open file in active JetBrains editor

│   ├── problems <file>              # Query IDE inspection errors and warnings

│   ├── symbols <query>              # Search symbols in JetBrains PSI tree

│   ├── build                        # Trigger IDE background build

│   └── run <config>                 # Execute an IDE run configuration

│

├── analyze                          # Code & Binary Intelligence

│   └── repo <git-url | path>        # Deep AST, complexity, and call-graph analysis

│       ├── --ephemeral              # Run analysis inside throwaway hardened container

│       ├── --depth <n>              # Community detection cluster depth

│       └── --format <text|json>     # Structured output format

│

├── sandbox                          # Ephemeral Sandboxed Execution

│   └── run [flags] <image> [cmd]    # Spawns isolated, cap-dropped container with RAII drop

│       ├── --ephemeral              # Strictly enforces zero persistent footprint

│       ├── --net <none|host|mesh>   # Network isolation mode (default: none)

│       └── --timeout <duration>     # Execution deadline

│

├── bridge                           # Model Context Protocol (MCP) Bridges

│   └── rea                          # Launch REA SSH/stdio bridge to remote Ghidra

│

├── supervisor                       # Run token & thermal governance batch daemon

│   ├── --mode <batch|hard|curfew>   # Select operational mode

│   └── --spend-cap <usd>           # Override financial ceiling

│

└── share                            # Configuration Distribution & Team Onboarding

├── export                       # Bundle configuration with encrypted secrets

└── import <bundle-file>         # Import and reconcile environment bundle

---

## 3. Subcommand Specifications & Ergonomics

### `dendrite env `

Inspects, replicates, and provisions the development environment either in Docker containers or directly on the host computer:

# 1. Audit target machine capabilities (OS, arch, package manager, installed tools)

dendrite env doctor

# 2. Plan containerized replication with Linear issue tracker

dendrite env plan --mode container --with-linear

# 3. Plan direct host installation

dendrite env plan --mode host

# 4. Replicate environment in Docker containers

dendrite env replicate --mode container

# 5. Install a specific tool on the host (e.g. Linear CLI)

dendrite env install linear --mode host

# 6. Install Linear in a container

dendrite env install linear --mode container

# 7. Check real-time health and status of all components

dendrite env status

---

### `dendrite doctor`

Inspects all local and remote dependencies across the active scenario:

dendrite doctor

**Checks Performed:**

- Docker daemon reachability (macOS local, remote Debian, Azure, and AWS SSH sockets).
- Tailscale mesh connectivity and node ping latency (`debian-server.ts.net`, `azure-burst.ts.net`, `aws-burst.ts.net`).
- Local Ollama Metal GPU availability and model weights check (`qwen2.5-coder:14b`).
- LiteLLM Proxy Gateway health (`/health/liveliness`) and PostgreSQL/Redis connection.
- Active issue tracker connectivity (Jira / Plane / Linear API status).
- Disassembler availability (Local Hopper binary path / Remote Ghidra headless executable).

---

### `dendrite init`

Sets up workstation configuration and creates default directory structures:
dendrite init --scenario local-hybrid --control-plane debian-server.ts.net
```

---

### `dendrite up [scenario]` & `dendrite down`

Deploys or reconciles the environment based on Terraform placement maps and Docker Compose profiles:
# Launch Scenario 1 (Debian Control Plane + Local macOS)
dendrite up local-hybrid

# Burst compute to Azure VM

dendrite up azure-burst

# Burst compute to AWS Spot EC2 instance (see FEAT-007)

dendrite up aws-burst

# Teardown active stacks

dendrite down

---

### `dendrite ui` (Web Dashboard &amp; Configurator)

Spins up the embedded web dashboard and opens the default browser:

dendrite ui --port 3000 --open-browser</code></pre>
