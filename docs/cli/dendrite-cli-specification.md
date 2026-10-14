# Dendrite CLI Specification & Command Reference

The `dendrite` CLI is the unified control plane binary for managing multi-node deployment scenarios, token governance, multi-project workspaces, issue tracker providers (Jira, Plane, Linear), AI-assisted code optimization, polyglot translation, documentation generation/reflection, and ephemeral sandboxed analysis.

---

## 1. CLI Architecture & Design Patterns

Implemented in idiomatic Go (with GoLand native ergonomics) or Rust, the CLI adheres to strict architectural patterns:

1. **Typestate Lifecycle Transitions**:
   - Commands enforce valid state transitions at compile-time/runtime:
     `Workspace<Unconfigured>` $\xrightarrow{\text{init}}$ `Workspace<Configured>` $\xrightarrow{\text{switch}}$ `Workspace<Active>`.
2. **RAII & Resource Cleanup Semantics**:
   - Ephemeral commands use deferred teardown traps (`defer c.Cleanup()` or `Drop`) to guarantee container removal, network detachment, and tmpfs unmounting even during runtime panics or `SIGINT` (Ctrl+C).
3. **Strategy Pattern for Pluggable Providers**:
   - Issue tracker implementations (Jira, Plane, Linear, GitHub Issues) adhere to a decoupled interface (`IssueTracker`), allowing frictionless provider switching via CLI flags or config.
4. **Intermediate Semantic Representation (ISR)**:
   - Polyglot translation and capability import decouple source syntax from target synthesis via typed Tree-sitter AST nodes.
5. **Domain Newtypes & Type Safety**:
   - Wrapping primitive IDs into dedicated types (`type ProjectID string`, `type VirtualKey string`, `type MeshNode string`, `type IssueID string`).
6. **Complexity Guardrails**:
   - Every CLI subcommand method is bounded: Cyclomatic Complexity $\le 10$, Cognitive Complexity $\le 15$, LOC per function $\le 60$.

---

## 2. Command Hierarchy & Syntax Matrix

```
dendrite [global flags] <subcommand> [args...]
```

```
├── doctor                           # Pre-flight environment diagnostics
├── init                             # Guided bootstrap and scenario setup
├── up [scenario]                    # Apply topology & launch active profile stacks
│                                    # (local-hybrid | azure-burst | aws-burst | distributed)
├── down                             # Gracefully halt active stacks
├── status                           # System health, spend dashboard & thermal monitor
├── ui [flags]                       # Launch Dendrite Web UI & Configuration Dashboard
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
```

---

## 3. Subcommand Specifications & Ergonomics

### `dendrite doctor`
Inspects all local and remote dependencies across the active scenario:
```bash
dendrite doctor
```
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
```bash
dendrite init --scenario local-hybrid --control-plane debian-server.ts.net
```

---

### `dendrite up [scenario]` & `dendrite down`
Deploys or reconciles the environment based on Terraform placement maps and Docker Compose profiles:
```bash
# Launch Scenario 1 (Debian Control Plane + Local macOS)
dendrite up local-hybrid

# Burst compute to Azure VM
dendrite up azure-burst

# Burst compute to AWS Spot EC2 instance (see FEAT-007)
dendrite up aws-burst

# Teardown active stacks
dendrite down
```

---

### `dendrite ui` (Web Dashboard & Configurator)
Spins up the embedded web dashboard and opens the default browser:
```bash
dendrite ui
```

---

### `dendrite tracker` (Pluggable Issue Tracker Management)
Allows seamless switching and configuration of project management backends:
```bash
# Switch to Jira and verify connection
dendrite tracker switch jira
dendrite tracker test
```

---

### `dendrite ai optimize-tokens`
Applies AST semantic compaction and enables context caching to minimize token burn:
```bash
dendrite ai optimize-tokens --target-reduction 60%
```
**Output Metrics:**
- Pre-compaction vs post-compaction token volume.
- LiteLLM context cache warming status.
- Projected monetary savings per 100 queries.

---

### `dendrite ai import` (Cleanroom Capability Import)
Dissects an external library or repository and synthesizes an idiomatic native implementation:
```bash
dendrite ai import \
  --source https://github.com/hyperium/http.git \
  --symbol HeaderMap \
  --target-lang go \
  --out pkg/http/headermap.go
```
Generates matching unit and property tests verifying behavioral parity.

---

### `dendrite translate` (Polyglot Language Translation)
Translates source files between programming languages with behavioral verification:
```bash
dendrite translate \
  --from python \
  --to rust \
  --source src/tokenizer.py \
  --out crates/tokenizer/src/lib.rs
```
Automatically generates dual property-based tests asserting identical input/output parity.

---

### `dendrite doc gen` & `dendrite doc reflect`
Manages Docs-as-Code generation and cross-project documentation mirroring:
```bash
# Generate FEAT-* and ADR-* docs from code and tests
dendrite doc gen --type feat

# Reflect documentation structure from a mature reference project into target
dendrite doc reflect \
  --reference ../reference-core-platform \
  --target .
```

---

### `dendrite status`
Outputs real-time terminal dashboard: active scenario, token spend (24h/7d), SoC die temperature, active tracker, and active workspace.

---

### `dendrite project` (Multi-Project Workspaces)
```bash
# Register, list, and switch project contexts
dendrite project add my-service git@github.com:org/my-service.git
dendrite project switch my-service
```

---

### `dendrite analyze repo` & The Ephemeral Flag (`--ephemeral`)
```bash
# Analyze a repository in an ephemeral throwaway container
dendrite analyze repo https://github.com/tokio-rs/tokio.git --ephemeral --format json
```

---

### `dendrite sandbox run --ephemeral`
```bash
# Ephemeral command execution with guaranteed RAII drop
dendrite sandbox run --ephemeral --image alpine:latest -- uname -a
```

---

### `dendrite bridge rea`
```bash
dendrite bridge rea --backend hopper
```

---

### `dendrite share export` & `dendrite share import`
```bash
dendrite share export --project my-service --recipient-key age1... --out my-service.bundle
dendrite share import my-service.bundle
```
