# Developer Test Automation, Self-Healing Harness & UI Integration Suite

# FEAT-023: Developer Test Automation, Self-Healing Harness & UI Integration Suite

## 1. User Story & Business Objective

As a software engineer and full-stack developer working on Dendrite, I need an automated testing tool (`dendrite test auto` and `dendrite test ui`) that eliminates the friction of writing comprehensive test suites. This tool must:

1. Automatically scaffold and synthesize idiomatic unit, integration, and Testcontainers-backed tests by extracting AST symbol boundaries via Tree-sitter ([FEAT-004](file:///Users/yasuomaidana/GoLandProjects/Dendrite/docs/features/FEAT-004-repo-intel-ast.md)) and querying local inference models (such as `oxillama` or `qwen2.5-coder:14b` from [FEAT-020](file:///Users/yasuomaidana/GoLandProjects/Dendrite/docs/features/FEAT-020-pluggable-inference-engines.md)).

2. Provide a **self-healing test loop** that runs tests, captures compiler diagnostics or assertion failures, and automatically refines the test code until green.

3. Provide an automated **UI & Integration Testing Harness** for the Dendrite Web UI ([FEAT-009](file:///Users/yasuomaidana/GoLandProjects/Dendrite/docs/features/FEAT-009-dendrite-web-ui.md)) using **k6-browser** and headless browser drivers to test end-to-end user journeys (e.g., scenario switching, spend limit updates, real-time WebSocket telemetry validation) under realistic load.

---

## 2. Technical Boundaries & Specifications

1. **AST-Driven Automated Test Synthesis (`pkg/testgen`)**:

- Parses target Go or Rust source files using Tree-sitter AST extractors.
- Detects external persistence dependencies (PostgreSQL queries, Redis keys, SQLite tables, HTTP clients) and automatically emits scaffolding with [Testcontainers](https://rust.testcontainers.org) to strictly satisfy the project Definition of Done (DoD).
- Generates table-driven test cases covering both happy path and failure/edge cases.

2. **Self-Healing Test Convergence Loop**:

- Executes the synthesized test via `go test` or `cargo test`.
- On compilation error or failed assertion, captures the diff and compiler stderr.
- Feeds the error context back to the local inference engine (`qwen2.5-coder:14b` via LiteLLM) in a constrained feedback loop (maximum 3 iterations).
- Validates that the final test achieves $\ge 85\%$ line/branch coverage without flaky behavior.

3. **UI Integration Testing Harness (`pkg/uitest`)**:

- **k6-browser Hybrid Load & Frontend Testing**:
- Simulates concurrent browser sessions interacting with the Dendrite Web UI (`:3456`).
- Validates frontend DOM rendering, Web Vitals (LCP, FID, CLS), and memory leaks during high-frequency WebSocket telemetry bursts (100 messages/sec).
- **Headless E2E Journeys**:
- Automates end-to-end workflows:

1. Login / session authentication over Tailscale mesh.

2. Switching topology scenario (e.g. from `local-hybrid` to `azure-burst`) and verifying dynamic pre-flight indicator state.

3. Updating token spend caps and verifying atomic writeback to `~/.dendrite/config.yaml`.

4. Triggering an agent run and validating streaming output cards in the Web UI.

5. Managing the Distributed Codebase Ecosystem & Interoperability control panel (FEAT-026: source priority reordering, interactive capability probing, modular REA decomposition DAG rendering, fallback lifecycle diff inspection, and Plane v2 satellite project creation).

4. **Developer CLI Workflow**:
   # Automatically scaffold and verify tests for a specific package or file
dendrite test auto --file pkg/gateway/limiter.go --with-testcontainers

# Run automated Web UI integration &amp; visual tests

dendrite test ui --headless --base-url http://127.0.0.1:3456

# Hybrid load and UI browser test

dendrite test ui --vus 10 --duration 45s --browser-metrics

```

</code></pre>

mermaid

flowchart TD

User["Developer: dendrite test auto"] --> Extractor["AST Boundary Extractor<br/>(Tree-sitter)"]

subgraph "Test Synthesis & DoD Enforcement"

Extractor --> Classifier["Dependency Classifier<br/>(Detects DB/Redis/HTTP)"]

Classifier --> Prompt["Prompt Synthesizer<br/>(Injects Testcontainers Boilerplate)"]

Prompt --> Inference["Local Inference Engine<br/>(FEAT-020: Oxillama / Ollama)"]

Inference --> TestFile["Generated Test File<br/>(*_test.go / tests/*.rs)"]

end

subgraph "Self-Healing Loop (Max 3 Iterations)"

TestFile --> Runner["Test Runner<br/>(go test / cargo test)"]

Runner -->|Failure / Compiler Error| Healing["Self-Healing Feedback Agent"]

Healing -->|Fix Patch| TestFile

Runner -->|Pass & Coverage >= 85%| Done["Verified Test Suite (DoD Passed)"]

end

subgraph "UI & Integration Harness (FEAT-009)"

WebUIRunner["UI Test Harness<br/>(k6-browser / Playwright)"]

WebUIRunner --> WebUI["Dendrite Web UI (:3456)"]

WebUI --> WS["WebSocket Telemetry Stream"]

WebUIRunner --> UIMetrics["Web Vitals, DOM State & Coverage"]

end

```

---

## 3. Idiomatic Design Patterns & Complexity Guardrails

1. **Typestate Pattern (Self-Healing Lifecycle)**:

```rust

// Typestate representation for test synthesis session

struct TestSession<Drafting>;

struct TestSession<Compiling> { test_code: String };

struct TestSession<Executing> { binary_path: PathBuf };

struct TestSession<Healing> { error_log: String, iteration: u8 };

struct TestSession<Verified> { coverage_pct: f32 };

```

2. **Strategy Pattern (UI Automation Drivers)**:

- `UIDriver` interface supporting `K6BrowserDriver`, `HeadlessChromiumDriver`, and `MockDriver`.

3. **RAII & Drop Semantics**:

- Browser processes, temporary test output files, and throwaway containers are cleaned up via `defer` / `Drop` upon execution exit.

4. **Domain Newtypes**:

- `type TestRunID string`
- `type IterationCount uint8`
- `type CoveragePercent float32`

5. **Complexity Thresholds**:

- Maximum Cyclomatic Complexity $\le 10$ per function.
- Maximum Cognitive Complexity $\le 12$ per function.
- Clean separation of concern between AST parsing, prompt construction, and test execution.

---

## 4. Acceptance Criteria & Verification Matrix

- [ ] **AC-1**: `dendrite test auto` analyzes AST symbols of a source file and synthesizes a compiling table-driven test suite.
- [ ] **AC-2**: Persistent dependencies (SQL queries or Redis clients) trigger automatic generation of Testcontainers initialization and teardown code in compliance with project DoD.
- [ ] **AC-3**: Self-healing loop detects syntax or assertion errors and resolves them within $\le 3$ iterations without manual intervention.
- [ ] **AC-4**: `dendrite test ui` launches headless k6-browser sessions against Web UI (`:3456`), executing scenario toggling and config updates with assertions on DOM state.
- [ ] **AC-5**: Web UI telemetry integration tests verify that 100 WebSocket messages/sec do not produce memory leaks or UI frame drops below 30 FPS.

---

## 5. Definition of Done (DoD) & Persistence Verification

- **Automated Tests**: Unit tests verify AST dependency classification and self-healing prompt formatting ($\ge 85\%$ coverage).
- **Persistent Layer Verification**: Test generator outputs verified using Testcontainers PostgreSQL and Redis containers.
- **Zero Linter Warnings**: `golangci-lint run` passes with zero issues.
