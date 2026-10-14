# Scrum Master Alignment & Pattern Complexity Scorecard

# Scrum Master Alignment & Pattern Complexity Scorecard

# Scrum Master Alignment & Pattern Complexity Scorecard

## 1. Persona & Governance Mandate

This document establishes the quality gates and engineering standards enforced by the **Agile Scrum Master & Goal Alignment Specialist** (`/scrum-master-alignment`) and the **Rust/Go Design Patterns & Code Complexity Auditor** (`/pattern-complexity-auditor`).

---

## 2. Definition of Done (DoD) Checklist

An architectural deliverable, feature pull request, or CLI subcommand is considered **Done** if and only if it strictly satisfies the following checklist:

- [ ] **Goal & Spec Traceability**: Every requirement specified in `FEAT-*` is mapped to at least one automated unit, integration, or BDD specification test.
- [ ] **Persistent Layer Testing via Testcontainers**:

> [!IMPORTANT]

> Any persistent layer feature (PostgreSQL 16 spend tables, Redis 7 sliding window counters, SQLite AST databases, MinIO artifact storage, LocalStack AWS emulation, WireMock API emulation) **MUST** include automated integration tests using [Testcontainers](https://rust.testcontainers.org) (`testcontainers`). Testing persistent layers solely via in-memory mocks is a violation of the DoD.

- [ ] **Code Coverage Threshold**: Line and branch coverage meets or exceeds **$\ge 85\%$** (`cargo llvm-cov` or `go test -cover`).
- [ ] **Zero Linter Warnings**:
- Go: `golangci-lint run` passes with zero issues.
- Rust: `cargo clippy --all-targets --all-features -- -D warnings` passes with zero warnings.
- [ ] **Zero Hidden Panics**: No unchecked `unwrap()`, naked indexing (`slice[i]`), or production panics. All errors are propagated via typed domain error enums (`thiserror`) or wrapped errors.
- [ ] **Clean Drop / RAII Semantics**: All ephemeral sandboxes, temporary files, and network sockets implement automatic cleanup upon exit or interruption.
- [ ] **Performance & SLA Guardrails**: Latency thresholds ($p99 < 350\text{ms}$, gateway cascade failover $< 2\text{s}$, thermal warning at $85^\circ\text{C}$) are enforced via automated k6 benchmark pipelines.

---

## 3. Requirement-to-Test Traceability Matrix

| Feature ID | Business Capability | Target Component | Verifying Test Suite | Persistence Verification |

| :--- | :--- | :--- | :--- | :--- |

| **FEAT-001** | Multi-Scenario Topology Placement | Terraform & Docker Compose | `tests/integration/test_topology.go` | Testcontainers Docker Engine |

| **FEAT-002** | Token Budget Governance & Fallbacks | LiteLLM Proxy & Gateway | `tests/integration/test_budget_gate.py` | Testcontainers PostgreSQL 16 & Redis 7 |

| **FEAT-003** | Reverse Engineering Suite (REA) | Hopper / Headless Ghidra Worker | `tests/integration/test_rea_bridge.go` | Testcontainers Headless Worker |

| **FEAT-004** | Repository AST Intelligence | Graphify Tree-sitter Parser | `tests/unit/test_ast_graph.go` | SQLite Persistent DB Test |

| **FEAT-005** | Multi-Project Workspaces | Dendrite Workspace Registry | `tests/unit/test_workspaces.go` | Local Config & DB Isolation Test |

| **FEAT-006** | Ephemeral Sandboxing | Sandboxed Container Runner | `tests/integration/test_ephemeral_raii.go`| Zero Residual Container Verification |

| **FEAT-007** | AWS Cloud Burst Provider | Terraform AWS EC2/EBS Module | `tests/integration/test_aws_burst.go` | Testcontainers LocalStack EC2/EBS |

| **FEAT-008** | Pluggable Issue Trackers (Jira) | Tracker Strategy & Adapters | `tests/integration/test_jira_adapter.go`| Testcontainers WireMock Jira v3 |

| **FEAT-009** | Dendrite Web UI & Dashboard | Embedded HTTP & Telemetry Engine | `tests/integration/test_web_ui.go` | Real-time WebSocket & Config Test |

| **FEAT-010** | AI Token Optimization & Import | Compaction & Import Synthesizer | `tests/unit/test_token_opt.go` | Testcontainers Redis Cache Test |

| **FEAT-011** | Polyglot Translation Engine | Tree-sitter ISR Code Synthesizer | `tests/integration/test_polyglot.go` | Dual Property Test Verification |

| **FEAT-012** | Docs Generation & Reflection | Spec Extractor & Reflection Mirror| `tests/integration/test_doc_reflect.go` | Docusaurus Build Container Test |

| **FEAT-013** | Autonomous Coding Agent Harness | Aider, OpenHands, OpenCode Harness | `pkg/agent/runner_test.go` | Testcontainers SQLite/Postgres Session Store |

| **FEAT-014** | JetBrains IDE MCP Integration | IDE MCP Bridge & Client | `pkg/jetbrains/jetbrains_test.go` | JetBrains MCP JSON-RPC Test |

| **FEAT-019** | Azure Cloud Burst Provider | Terraform Azure Resource Module | `tests/integration/test_azure_burst.go` | Terraform Inspec / Mock SSH Test |

| **FEAT-020** | Pluggable Inference Engines | Multi-Engine Gateway Adapters | `pkg/inference/inference_test.go` | Testcontainers Mock OpenAI/Ollama HTTP |

| **FEAT-021** | Performance Testing & Load Harness | k6 Benchmarking Engine (`pkg/perf`)| `tests/perf/gateway_benchmark_test.go`| Testcontainers k6 Runner + PostgreSQL 16 |

| **FEAT-022** | Agent Search & Capability Discovery | Hybrid Vector & AST Indexer | `pkg/agentsearch/search_test.go` | Testcontainers TEI / SQLite Manifest Store |

| **FEAT-023** | Test Automation & UI Test Harness | Self-Healing TestGen & UI Driver | `pkg/testgen/testgen_test.go` | Testcontainers Headless Browser & PostgreSQL |

| **FEAT-026** | Distributed Codebase Ecosystem & Interoperability | Priority Resolver & Coordinator | `pkg/distcodebase/distcodebase_test.go` | Multi-Source Priority & Plane v2 Integration Test |

---

## 4. Software Design Patterns & Idiomatic Standards

### 1. Typestate Pattern (Lifecycle Safety)

Workspaces, remote connections, sandboxes, and benchmark sessions must enforce state transitions via types rather than runtime boolean flags:

// Idiomatic Typestate representation for Benchmark Execution

struct BenchmarkSession&lt;Uninitialized&gt;;

struct BenchmarkSession&lt;Provisioned&gt; { runner: K6Runner, scenario: ScenarioPlan }

struct BenchmarkSession&lt;Running&gt; { cancel_fn: Box&lt;dyn FnOnce()&gt;, start_time: Instant }

struct BenchmarkSession&lt;Completed&gt; { summary: BenchmarkSummary, metrics: MetricStream }

impl BenchmarkSession&lt;Uninitialized&gt; {

pub fn provision(self, plan: ScenarioPlan) -&gt; Result&lt;BenchmarkSession&lt;Provisioned&gt;, PerfError&gt; { /* ... */ }

}

impl BenchmarkSession&lt;Provisioned&gt; {

pub fn execute(self) -&gt; Result&lt;BenchmarkSession&lt;Running&gt;, PerfError&gt; { /* ... */ }

}</code></pre>

### 2. Strategy Pattern (Pluggable Issue Trackers, Engines & Test Drivers)

Decouples vendor-specific REST/GraphQL engines (Jira, Plane, Linear), agent runtimes (Aider, OpenHands, OpenCode, AGY), inference engines (Ollama, OxiLLaMa, vLLM, TGI), and UI automation drivers (k6-browser, Playwright):

type UIDriver interface {

Launch(ctx context.Context, targetURL string) error

Navigate(ctx context.Context, route string) error

AssertElementVisible(ctx context.Context, selector string) error

CaptureWebMetrics(ctx context.Context) (WebMetrics, error)

Close() error

}</code></pre>

### 3. Intermediate Semantic Representation (ISR) & AST-Driven Scaffolding

Decouples source language AST parsing from target test generation during automated test synthesis ([FEAT-023](file:///Users/yasuomaidana/GoLandProjects/Dendrite/docs/features/FEAT-023-developer-test-automation-ui-harness.md)).

### 4. RAII & Drop Semantics (Ephemeral Sandboxing & Test Runtimes)

Temporary resources, throwaway containers, browser contexts, and scratch mounts must be reclaimed deterministically using `Drop` (Rust) or `defer` (Go):

// Idiomatic Go RAII cleanup pattern for k6 runner and browser context

func RunUIBenchmark(ctx context.Context, cfg UIConfig) (Result, error) {

driver, err := browser.NewDriver(ctx, cfg)

if err != nil {

return Result{}, err

}

// Guarantees immediate cleanup even on panic, error, or SIGINT

defer func() {

_ = driver.Close()

_ = os.RemoveAll(cfg.ScratchDir)

}()

return driver.ExecuteJourney(ctx)

}</code></pre>

### 5. Domain Newtypes

Avoid stringly-typed APIs by wrapping raw primitives:

type ProjectID string

type VirtualKey string

type MeshNodeAddress string

type IssueID string

type AgentType string

type VirtualUsers uint32

type TTFT time.Duration

type TokensPerSecond float64

type BenchmarkRunID string</code></pre>

---

## 5. Complexity Thresholds & Anti-Patterns to Eliminate

### Quantitative Thresholds

- **Cyclomatic Complexity (CC)**: $\le 10$ per function (hard limit: 15).
- **Cognitive Complexity**: $\le 15$ per function (hard limit: 20).
- **Lines of Code (LOC)**:\
- Function: $\le 60$ lines (hard limit: 80).\
- File / Module: $\le 500$ lines (hard limit: 800; trigger module split).

### Anti-Patterns Flagged

- **God Structs & Monolithic Handlers**: Splitting large CLI state objects into isolated domain controllers (`TopologyController`, `WorkspaceController`, `TrackerController`, `UIController`, `AIEngineController`, `AgentController`, `PerfBenchmarkController`, `TestGenController`).
- **Excessive Cloning / Memory Allocation**: Favoring string slices (`&str`, `string_view`) and zero-copy streaming.
- **Leaked Ephemeral State**: Orphaned containers, hanging browser headless processes, or dangling bind mounts in `/tmp` post-execution.
