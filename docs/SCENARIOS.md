---
id: scenarios
title: "Dendrite — Use Cases, User Stories & Operational Scenarios"
sidebar_label: "Operational Scenarios"
sidebar_position: 2
tags: [scenarios, user-stories, workflows, jetbrains, pool, query, store, scheduler]
---

Dendrite — Use Cases, User Stories & Operational Scenarios (docs/SCENARIOS.md)

Repository: https://github.com/yasuomaidana/dendrite

Core Runtime: Go (cmd/dendrite, internal/pool, internal/query, internal/store, internal/scheduler)

Target IDEs: JetBrains RustRover, GoLand, WebStorm, and PyCharm

Architectural Pillars

Autonomous Pool DAG (dendrite pool): Automatically inspects manifests, AST exports, FFI boundaries, and IPC contracts to infer which project is a dependency of which. Whenever Dendrite determines a new project member should be created (e.g., splitting a shared library or scaffolding a merged workspace), it always prompts the user for the target directory in both the CLI and UI.

Universal Intent & Query Engine (dendrite query): Executes custom structural queries, feature slicing, subsystem swaps, language translations, and multi-binary/multi-repo merges—replacing rigid single-purpose commands while enforcing 3-Tier Library Reuse across Source References, Pool Members, and Ecosystem Registries.

Persistent Segmented Store (dendrite store): Analyzes binaries and repositories once in token-efficient chunks and persists the structural/semantic graph permanently until explicitly deleted by the user via CLI or UI.

Token-Aware Hybrid Scheduler (dendrite analyze): Conserves cloud LLM quotas (e.g., Gemini) by running zero-token AST passes and local night-time models first, automatically switching to paid tokens 48 hours before the weekly quota reset down to a 20% safety reserve—with user switches for a 0% drain override or continuous analysis until paused.

Zero-Data-Loss Infrastructure Mobility (dendrite node & dendrite backup): Migrates background services seamlessly between a Local Workstation, a Debian Server, and Azure Cloud, backed by a Smart Repo-Aware Backup Engine (2 versions by default, evicting Git-persisted blobs first when disk quota is reached).

Quick-Reference Command & UI Matrix

Subsystem	Primary CLI Command	Equivalent UI View (dendrite ui)	Core Responsibility

Onboarding	dendrite init [--ui] [--ide jetbrains]	Setup Wizard Modal	Initializes .dendrite/, .idea/ scopes, and initial config.

Stories	dendrite story start / fork / resume	Story Board & Timeline	Tracks architectural tasks, speculative branches, and ADRs.

Pool DAG	dendrite pool add / resolve / switch	Pool Topology Graph	Infers dependency edges and switches published/branch/local/fork.

Source Refs	dendrite ref add <repo-or-path>	Reference Library Drawer	Registers reference repos checked during --audit-library-reuse.

Custom Query	dendrite query [flags] "<prompt>"	Query Studio	Slices, translates, swaps subsystems, or merges binaries and repos.

Analysis Store	dendrite store status / inspect / rm	Storage & Chunk Inspector	Shows STORED vs. ANALYZING_PARTIAL projects; purges on demand.

Token Policy	dendrite analyze policy / mode / pause	Token Quota & Scheduler	Controls night local runs, 48h Gemini sprint, 20%/0% floor, and pause.

Node Mobility	dendrite node migrate --from X --to Y	Compute Nodes Topology	Moves services across Local \leftrightarrow Debian \leftrightarrow Azure without data loss.

Smart Backup	dendrite backup init / run / restore	Backup Slots & Quota Bar	Retains 2 snapshots by default; evicts Git-persisted objects first.

Documentation	dendrite docs sync / serve	Embedded Docs Portal	Generates and serves federated multi-project Docusaurus sites.

Configuration	dendrite config set / edit --editor nvim	Settings Inspector	100% parity across Web/TUI controls, CLI flags, and nvim/vim.

JetBrains IDE Integration Baseline

Across all scenarios, Dendrite synchronizes project boundaries and dependency overrides directly with your active JetBrains IDE:

JetBrains IDE	Ecosystem	Local Pool Sync Mechanism Managed by Dendrite	IDE Indexing & Context Hook

GoLand	Go (go.mod)	go.work (go work use) & replace directives	Automatic Go Workspace sync + External Tools watcher

RustRover	Rust (Cargo.toml)	[patch.crates-io], [patch."https://..."], & Workspace members	Automatic Cargo reload + .idea/scopes/ generation

WebStorm	TypeScript / JS (package.json)	pnpm-workspace.yaml, bun/npm link, or file: overrides	TypeScript Language Service restart + symlink loop exclusion

PyCharm	Python (pyproject.toml)	uv workspaces (tool.uv.sources) or editable pip install -e	Virtualenv refresh + Mark Directory as Sources Root

Part I: Onboarding, Initialization & Autonomous Pool Synchronization (Cases 1–5)

Case 1: Greenfield Onboarding — Install Dendrite & Start a Story in a New Project

User Story: As a new user with the link to [https://github.com/yasuomaidana/dendrite](https://github.com/yasuomaidana/dendrite), I want to install the tool via Homebrew, Go, or a pre-built release, initialize a brand-new project using either the CLI or optional UI, and launch my first development story inside my JetBrains IDE.

Target IDEs: GoLand, RustRover, WebStorm, PyCharm.

Workflow:

Install Dendrite (Choose One):

# Option A: Homebrew (macOS & Linux)

brew install yasuomaidana/tap/dendrite

# Option B: Go Toolchain ( directly from source)

go install github.com/yasuomaidana/dendrite/cmd/dendrite@latest

# Option C: Debian Server (.deb package from GitHub Releases)

curl -LO https://github.com/yasuomaidana/dendrite/releases/latest/download/dendrite_linux_amd64.deb

sudo apt install ./dendrite_linux_amd64.deb

Initialize a New Project (CLI or Interactive UI):

mkdir my-new-project && cd my-new-project

# CLI initialization with JetBrains scaffolding:

dendrite init --name "my-new-project" --ide jetbrains

# OR launch the interactive Local Web/TUI Setup Wizard:

dendrite init --ui

Start Your First Story:

dendrite story start "feat/core-architecture" \

--prompt "Scaffold domain types, persistence layer, and telemetry worker"

JetBrains Workflow: Open my-new-project in GoLand / RustRover / WebStorm / PyCharm. Dendrite generates .idea/runConfigurations/Dendrite_Sync_Story.xml and a custom file scope so you can execute and track story tasks directly from the IDE toolbar.

Case 2: Brownfield Adoption — Attach Dendrite to an Existing Project

User Story: As a developer who already installed Dendrite, I want to integrate it into an existing, active repository without altering its Git history or overwriting existing .idea configurations, and start a story scoped to a specific subsystem.

Target IDEs: Any JetBrains IDE where the project is already open.

Workflow:

Attach Dendrite from the JetBrains Integrated Terminal (Alt+F12):

dendrite attach --ide jetbrains

Zero-Token Structural Pass: Dendrite runs its fast Go tree-sitter pass to map modules, packages, and manifests into dendrite store without consuming cloud tokens.

Start a Scoped Story:

dendrite story start "refactor/zero-alloc-parser" \

--scope "internal/parser/**" \

--prompt "Eliminate heap allocations in the frame decoder hot path"

JetBrains Workflow: Switch the JetBrains Project View dropdown to the Dendrite Active Story scope to focus exclusively on the files affected by the story.

Case 3: Reference-Driven Genesis — Start a Project from Source References & a Prompt

User Story: As an architect starting a new project, I want to register one or more external repositories as Source References and provide a prompt so Dendrite synthesizes a new project that reuses proven patterns and libraries from those references.

Target IDEs: RustRover, GoLand, WebStorm, PyCharm.

Workflow:

Register Source References in the Persistent Store:

dendrite ref add https://github.com/org/reference-raft-engine --alias raft-ref

dendrite ref add https://github.com/org/reference-webui --alias webui-ref

Bootstrap the New Project Using a Custom Query:

dendrite new ./distributed-kv \

--refs "raft-ref,webui-ref" \

--ide jetbrains \

--prompt "Create a distributed key-value coordinator in Go using the WAL framing pattern from raft-ref and the WebSocket streaming contract from webui-ref."

JetBrains Workflow: Open ./distributed-kv in GoLand and WebStorm. The reference repositories are attached as Read-Only External Libraries so you can Cmd+Click into reference structs without polluting your repository's build graph.

Case 4: Autonomous Multi-Directory Synchronization via dendrite pool (Published $\rightarrow$ Branch $\rightarrow$ Local $\rightarrow$ Fork)

User Story: As a developer with Project A and Project B in two different directories, I want to add both to dendrite pool, let Dendrite automatically determine that Project A depends on Project B, and seamlessly transition their link across a published version, a remote Git branch, live local edits, or a custom fork while keeping my JetBrains IDE synchronized.

Target IDEs: RustRover (Cargo.toml), GoLand (go.work/go.mod), WebStorm (package.json), PyCharm (pyproject.toml).

Step-by-Step Pool Lifecycle Workflow

Add Directories to the Pool & Let Dendrite Infer the Dependency Graph:

dendrite pool add /workspace/project-a /workspace/project-b

dendrite pool resolve

Output:

[Pool DAG Solver]

Inferred Dependency: project-a ──depends_on──> project-b (matched 14 imported symbols & manifest module path)

JetBrains IDE Sync: Linked project-a and project-b in workspace scope.

Stage 4A — Update Project A to a Newly Published Version of Project B:

After developing a feature in Project B and publishing v1.3.0 (to crates.io, GitHub tags/pkg.go.dev, npm, or PyPI):

dendrite pool switch project-b --in project-a --mode published --version "v1.3.0"

Action: Strips any temporary local overrides, pins v1.3.0 in Project A's manifest and lockfile, and refreshes the JetBrains package index.

Stage 4B — Switch Project A to Track an Experimental Remote Branch of Project B:

dendrite pool switch project-b --in project-a --mode branch --branch "feat/async-stream"

Action: Rewrites Project A's dependency manifest to track the remote Git branch feat/async-stream and updates the lockfile commit SHA.

Stage 4C — Drop Down to Live Local Synchronization (When Something is Missing):

You discover a missing trait/method in Project B while working in Project A and need to modify both locally at the same time:

dendrite pool switch project-b --in project-a --mode local

Action: Automatically injects a local [patch] in RustRover, go.work use directive in GoLand, workspace link: override in WebStorm, or editable uv source in PyCharm. Edits in /workspace/project-b immediately reflect in /workspace/project-a with zero publish lag.

Stage 4D — Switch to a Fork (When Upstream Project B Cannot Be Pushed Directly):

dendrite pool switch project-b --in project-a --mode fork \

--fork-url "https://github.com/yasuomaidana/project-b-fork" \

--push-branch "fix/custom-patch"

Action: Pushes your local Project B changes to your fork remote and rewires Project A's manifest to the fork URL so remote CI pipelines build reproducibly.

Case 5: Mid-Flight Project Split & Autonomous Pool Member Creation (With Mandatory Directory Prompt)

User Story: As a developer working inside a project (or across a pool), I realize part of my current code needs to live in a brand-new standalone project—or Dendrite's pool analyzer autonomously detects that a shared subsystem should be spun out into a new pool member. In both CLI and UI, Dendrite must ask me for the target directory before creating the new project and wiring live synchronization.

Target IDEs: RustRover, GoLand, WebStorm, PyCharm.

Workflow:

Autonomous Proposal or Explicit Split Request:

# Option A: Let the pool analyzer evaluate and propose new members

dendrite pool optimize

# Option B: Explicitly request splitting a module into a new pool member

dendrite pool split --from /workspace/project-a --module "internal/graph_matcher" --name "embedag-matcher"

Mandatory Target Directory Gate (CLI & UI):

In the CLI:

[Dendrite Pool] New pool member `embedag-matcher` will be created and linked as a dependency of `project-a`.

? Specify the target directory for `embedag-matcher` (required):

> /workspace/embedag-matcher

In the UI (dendrite ui): Opens the Create Pool Member Modal, displaying the inferred dependency edges (project-a -> embedag-matcher) and requiring you to select or type the destination directory via the Directory Picker before proceeding.

Automated Extraction & Lockstep Wiring:

Dendrite creates /workspace/embedag-matcher, moves the AST slice and its unit tests, initializes its manifest (go.mod / Cargo.toml / package.json / pyproject.toml), registers it in dendrite pool in local lockstep mode, and attaches it to your active JetBrains IDE window.

Part II: Binary-Driven Engineering via dendrite query (Cases 6–9)

Unified Query Architecture: Binary engineering workflows use dendrite query. Before analyzing a binary, Dendrite checks dendrite store by BLAKE3 hash; if already analyzed, it executes the query immediately against the stored Control Flow Graph (CFG), symbol table, and semantic index.

Case 6: Binary Debloating — Reconstruct Only the Functionality of Interest from a Binary

User Story: As an engineer improving or reverse-engineering a bloated binary, I want to strip out 90% of unneeded features (GUI, telemetry, legacy updaters) and reconstruct a clean, standalone project containing only the core functionality of interest.

Target IDEs: RustRover, GoLand, PyCharm.

Workflow:

dendrite query \

--sources "./legacy_controller.bin" \

--entrypoint "process_canbus_frame" \

--audit-library-reuse \

"Extract only the CAN-bus frame parser, state machine, and CRC16 routine; strip all GUI, updater, and logging subsystems into a standalone project."

Execution: Dendrite performs forward/backward CFG slicing from process_canbus_frame, prompts you for the new project target directory via dendrite pool, and generates clean source code + golden I/O test vectors inside RustRover or GoLand.

Case 7: Binary Subsystem Swap — Replace a Core Engine (e.g., Graphics Engine) Based on a Binary

User Story: As a developer modernizing an application from its binary, I want to keep its core simulation/domain behavior intact while replacing its legacy graphics engine (e.g., OpenGL/DirectX) with a modern rendering engine (wgpu in RustRover or Three.js/WebGPU in WebStorm).

Target IDEs: RustRover (wgpu/iced), WebStorm (Three.js).

Workflow:

dendrite query \

--sources "./legacy_sim.exe" \

--audit-library-reuse \

"Preserve the rigid-body kinematics and scene graph update loop, but replace all OpenGL rendering calls with a modern wgpu render pipeline."

Execution: Dendrite isolates the boundary between simulation state updates and opengl32 FFI calls, generates a clean RenderBackend trait boundary, and wires a wgpu renderer crate while keeping the original simulation step logic intact.

Case 8: Binary-to-Rust Translation — Port a Binary to Idiomatic Rust with Source-Ref & Crate Reuse

User Story: As a Rust engineer translating a compiled C/C++/Go/Python binary into safe Rust inside RustRover, I want Dendrite to check my registered Source References, Pool Members, and crates.io so it reuses existing libraries instead of translating decompiled library boilerplate.

Target IDE: RustRover.

Workflow:

dendrite query \

--sources "./autopilot_firmware.elf" \

--to rust \

--audit-library-reuse \

"Translate the attitude estimation and PWM motor mixer into safe, idiomatic Rust. Reuse our registered source references and standard Rust crates wherever possible."

Library Reuse Audit in Action:

Matches decompiled matrix/quaternion routines in autopilot_firmware.elf against nalgebra on crates.io and your local Source References (stm32-hal, foc-sdk).

Replaces 12,000 lines of decompiled math and HAL boilerplate with use nalgebra::*; and direct imports from your registered Source References.

Translates only the proprietary domain control laws into idiomatic Rust and generates a cargo test differential harness in RustRover.

Case 9: Binary Fusion — Merge Two Projects Based on Their Binaries via dendrite query

User Story: As a systems integrator with two separate compiled binaries (binary_a and binary_b), I want to merge their capabilities into a unified project using dendrite query, deduplicating shared libraries and letting dendrite pool prompt me for the new merged project directory.

Target IDEs: RustRover, GoLand.

Workflow:

dendrite query \

--sources "./sensor_ingest.bin,./trajectory_solver.bin" \

--to rust \

--audit-library-reuse \

"Merge the high-frequency sensor ring-buffer from sensor_ingest.bin with the trajectory optimizer from trajectory_solver.bin into a single unified workspace using zero-copy in-process channels."

Execution:

Dendrite inspects both binaries from dendrite store, identifies overlapping static libraries (e.g., both binaries compiled in protobuf and linear algebra routines), and consolidates them using --audit-library-reuse.

Replaces slow inter-process socket/pipe serialization with an in-process ring buffer (crossbeam-channel or Go channel).

Prompts you in the CLI or UI for the destination directory of the new merged pool member and opens the workspace in RustRover or GoLand.

Part III: Repository-Driven Engineering via dendrite query (Cases 10–13)

Cases 10–13 execute the same transformations as Cases 6–9, but operate directly on Source Repositories using AST call graphs, type hierarchies, and manifest analysis.

Case 10: Repository Feature Slicing — Strip Bloat and Keep Only Target Functionality (Repo Equivalent of Case 6)

User Story: As a developer working with a massive repository, I want to extract only a specific feature or algorithm into a lean project while automatically pruning dead code and unneeded manifest dependencies.

Target IDEs: GoLand, RustRover, WebStorm, PyCharm.

Workflow:

dendrite query \

--sources "https://github.com/upstream/monolithic-suite" \

--audit-library-reuse \

"Keep only the incremental DAG similarity matcher and its unit tests; remove all HTTP server, CLI, and database migration modules."

Execution: Dendrite traces the AST reachability graph from the DAG matcher, strips 85% of unrelated dependencies from Cargo.toml / go.mod / package.json / pyproject.toml, replaces vendored helpers with your existing Source References or Pool libraries, and prompts for the target directory if creating a new pool member.

Case 11: Repository Subsystem Migration — Swap a Core Engine such as the Graphics Engine (Repo Equivalent of Case 7)

User Story: As a developer improving a source repository, I want to swap out its graphics or rendering engine (e.g., migrating from Canvas2D/PixiJS to Three.js in WebStorm, or from glium to wgpu in RustRover) while keeping all domain logic and state management untouched.

Target IDEs: WebStorm, RustRover, GoLand, PyCharm.

Workflow:

dendrite query \

--sources "/workspace/cad-designer" \

--audit-library-reuse \

"Migrate the viewport rendering subsystem from legacy WebGL/PixiJS to Three.js while preserving the Svelte 5 state store and geometry kernel."

Execution: Dendrite checks registered Source References first to see if you already have a Three.js viewport wrapper in another project, generates the adapter layer, and creates an ordered refactoring story in WebStorm.

Case 12: Repository-to-Rust Porting — Translate a Source Repository to Rust with Reuse Verification (Repo Equivalent of Case 8)

User Story: As an engineer porting a Python (PyCharm), Go (GoLand), or TypeScript (WebStorm) repository into Rust (RustRover), I want Dendrite to translate the core architecture while mapping source-language libraries directly to my registered Source References, local Pool crates, and crates.io.

Target IDEs: PyCharm / GoLand / WebStorm (Source) $\rightarrow$ RustRover (Target).

Workflow:

dendrite query \

--sources "/workspace/python-occupancy-grid" \

--to rust \

--audit-library-reuse \

"Translate this Python occupancy grid and raycasting repository into an idiomatic Rust crate, reusing our local pool crates and crates.io equivalents for linear algebra and serialization."

Case 13: Multi-Repository Architecture Fusion — Merge Two Repositories via Custom Query (Repo Equivalent of Case 9)

User Story: As an architect merging two separate source repositories into a unified workspace, I want Dendrite to deduplicate overlapping internal packages, unify dependency versions, and prompt me for the new workspace directory.

Target IDEs: GoLand, RustRover, WebStorm, PyCharm.

Workflow:

dendrite query \

--sources "/workspace/doc-indexer,/workspace/graph-engine" \

--audit-library-reuse \

"Merge doc-indexer and graph-engine into a unified workspace, consolidating duplicate error/config modules and aligning shared dependencies against our source references."

Part IV: Hybrid Analysis & 3-Tier Library Reuse Verification (Case 14)

Case 14: Hybrid Binary + Multi-Repository Analysis & Synthesis

User Story: As a systems analyst or reverse engineer, I want to analyze a compiled binary alongside multiple source repositories and registered Source References to pinpoint exact code matches, isolate proprietary binary deltas, and verify that our workspace reuses existing libraries with maximum efficiency.

Target IDEs: RustRover, GoLand, PyCharm, WebStorm.

Workflow:

dendrite query \

--sources "./proprietary_drive.elf,ref:stm32-hal,ref:foc-motor-lib,/workspace/our-controller" \

--audit-library-reuse \

"Compare proprietary_drive.elf against stm32-hal, foc-motor-lib, and our-controller. Identify which binary functions are already covered by our source references, extract the novel dead-time compensation algorithm from the binary, and audit our-controller for redundant library code."

How --audit-library-reuse Enforces the 3-Tier Reuse Hierarchy (Cases 8–14)

Whenever --audit-library-reuse is triggered, Dendrite executes a strict 3-tier lookup against dendrite store before emitting any code:

Priority Tier	Source Checked	Action Taken When a Match is Found

Tier 1: Source References (dendrite ref)	All repositories and reference libraries registered via dendrite ref add and stored in dendrite store.	Directly imports or slices the verified implementation from your Source Reference and records provenance in the story ADR.

Tier 2: Active Pool Members (dendrite pool)	All local projects registered in your active dendrite pool.	Wires an automatic dependency edge in dendrite pool (or proposes extracting a shared pool member, prompting you for the directory).

Tier 3: Ecosystem Registries	crates.io (Rust), pkg.go.dev (Go), npm (TS/JS), PyPI (Python).	Maps decompiled or foreign-language library patterns (e.g., Eigen, numpy, zlib, SQLite) to canonical target ecosystem packages (nalgebra, spade, serde, diesel, ent).

Part V: Persistent Analysis Store & Token-Saving Hybrid Scheduler (Cases 15–16)

Case 15: Persistent Segmented Analysis Store — Analyze Once, Keep Until Evicted (dendrite store)

User Story: As a developer working with large repositories, source references, and binaries, I want Dendrite to persist every analyzed artifact once so I never pay time or token costs twice for the same code. I also need clear visibility in both the CLI and UI into which projects are fully analyzed (STORED) versus partially analyzed (ANALYZING_PARTIAL), and the ability to manually delete stored analyses whenever I choose.

How Segmented Persistence Works

Content-Addressed Caching: Every file/function in a repository and every CFG block in a binary is hashed with BLAKE3 and stored in ~/.dendrite/store/ (SQLite metadata + compressed graph blobs).

Incremental Checkpointing: Because analysis runs in modular parts/chunks to save tokens, each completed chunk is immediately committed to dendrite store. Queries can immediately use all completed chunks even while a project is at 45% semantic completion.

Permanent Until User Purges: Stored analyses never expire automatically. They remain available across all projects and stories until you explicitly delete them via CLI or UI.

CLI & UI Commands for Store Inspection & Management

# View real-time status of all Stored, Partially Analyzed, Queued, and Paused projects

dendrite store status

Output:

STORE ID    ARTIFACT NAME           KIND        STATE                 PROGRESS       TOKENS   UPDATED

─────────────────────────────────────────────────────────────────────────────────────────────────────────────────

ref-a10f    stm32f4xx-hal           SourceRef   STORED                100% (92/92)    11.4k   2026-10-10

bin-c82e    legacy_controller.bin   Binary      STORED                100% (48/48)    39.2k   2026-10-12

pool-991b   embedag-engine          PoolMember  ANALYZING_PARTIAL      62% (31/50)     8.3k   Active (Chunk 32)

ref-77d4    threejs-cad-shell       SourceRef   QUEUED_NIGHT_LOCAL     25% (15/60)     0.0k   Scheduled 23:00

bin-30f1    simulator_v2.exe        Binary      PAUSED_QUOTA_FLOOR     80% (64/80)    54.0k   Paused at 20% Floor

# Inspect which specific modules/chunks of a project are analyzed vs. pending

dendrite store inspect pool-991b

# Permanently remove an analyzed project from the store (also available via the UI Delete button)

dendrite store rm bin-30f1

Case 16: Token-Aware Hybrid Scheduler — Night Local Runs, 48h Weekly Gemini Sprint, 20%/0% Floor & Pause Switch

User Story: As a power user optimizing LLM token consumption (e.g., using a weekly Gemini quota alongside local models), I want to define scheduling directives so Dendrite analyzes projects in parts: using local-only models at night during the week, automatically switching to paid/cloud Gemini tokens 2 days (48h) before the weekly quota resets down to a 20% safety reserve, providing a switch to allow draining down to 0%, and offering an always mode that analyzes continuously until I pause it.

State Machine of the Token-Aware Scheduler

┌────────────────────────────────────────────────────────────────────────────┐

│                   DENDRITE CHUNKED ANALYSIS SCHEDULER                      │

├────────────────────────────────────────────────────────────────────────────┤

│                                                                            │

│  [1. Zero-Token Pass (Immediate on `pool add` / `ref add`)]                │

│      • Runs native Go AST / CFG extraction immediately (0 LLM tokens).     │

│                                                                            │

│  [2. Policy Mode (`mode = "policy"`)]                                      │

│      ├─► More than 48h before Weekly Quota Reset:                          │

│      │   • Waits for Night Window (e.g., 23:00–06:00).                     │

│      │   • Executes semantic chunks using LOCAL model only (0 cloud tokens)│

│      │                                                                     │

│      └─► Within 48h (2 Days) of Weekly Quota Reset:                        │

│          • Activates Cloud Provider (e.g., Gemini) to accelerate pending   │

│            chunks so unused weekly quota is not wasted.                    │

│          • Stops automatically when remaining weekly quota reaches 20%     │

│            (unless `allow_zero_quota = true` switch is enabled -> 0%).     │

│                                                                            │

│  [3. Continuous Mode (`mode = "always"`)]                                  │

│      • Analyzes chunks continuously 24/7 using the active provider until   │

│        the user triggers `dendrite analyze pause` (or hits quota floor).   │

└────────────────────────────────────────────────────────────────────────────┘

CLI & UI Controls for the Scheduler

# 1. Configure the hybrid Night-Local + 48h Pre-Reset Gemini Sprint directive

dendrite analyze policy \

--cloud-provider gemini \

--local-provider ollama \

--night-window "23:00-06:00" \

--weekly-reset "SUN 23:59" \

--sprint-before-reset "48h" \

--reserve-floor 20%

# 2. Switch: Allow draining paid/cloud tokens all the way down to 0% (or restore 20% guard)

dendrite analyze switch --allow-zero on

dendrite analyze switch --allow-zero off

# 3. Switch: Toggle between Scheduled Policy Mode and "Always Analyzing Until Paused" Mode

dendrite analyze mode always

dendrite analyze mode policy

# 4. Pause or Resume analysis immediately (persists current chunk progress in `dendrite store`)

dendrite analyze pause

dendrite analyze resume

In the UI (dendrite ui $\rightarrow$ Scheduler & Quota Bar): Provides live visual gauges for Weekly Quota Remaining (%), Time Until Weekly Reset, a one-click toggle switch for Allow Drain to 0%, a mode selector (Policy Night/Sprint vs. Always Analyzing), and a global Pause / Resume Analysis button.

Part VI: Advanced Polyglot, Speculative & Drift Workflows (Cases 17–20)

Case 17: Polyglot Full-Stack & Embedded Synchronization Across All Four JetBrains IDEs

User Story: As an engineer building a system spanning Rust firmware (RustRover), a Go backend (GoLand), a TypeScript/Svelte/Three.js frontend (WebStorm), and Python analytics (PyCharm), I want dendrite pool to infer the cross-language contract graph and synchronize schema changes across all four IDEs in a single story.

Workflow:

dendrite pool add ./firmware-rs ./backend-go ./frontend-ts ./analytics-py

dendrite pool resolve

dendrite story start "feat/telemetry-v2-packet" --sync-contracts

Execution: dendrite pool detects shared Protobuf/JSON-Schema/OpenAPI/FFI definitions connecting the four projects and flags contract drift across RustRover, GoLand, WebStorm, and PyCharm simultaneously.

Case 18: Speculative Story Branching — Benchmark Competing Architectures Side-by-Side

User Story: As an engineer deciding between two competing libraries or algorithms, I want to fork a story into two isolated git worktree branches, benchmark both inside my JetBrains IDE, and merge the winner while saving the benchmark data in dendrite store.

Workflow:

dendrite story fork "perf/graph-traversal" --variants "astar-bitvec,bidirectional-dijkstra"

# Run benchmarks in GoLand / RustRover, then resolve:

dendrite story resolve "perf/graph-traversal" --winner "bidirectional-dijkstra"

Case 19: Upstream Drift & Breaking-Change Reconciliation

User Story: As a developer maintaining a project sliced or translated from an upstream repository or Source Reference (Cases 3, 10, 12, 14), I want Dendrite to check upstream commits, filter out changes to modules we stripped away, and generate a story to port only relevant upstream bug fixes.

Workflow:

dendrite ref sync --check-drift

dendrite query --sources "ref:upstream-repo" "Port upstream security and memory fixes from v1.4.0..v1.4.3 into our sliced local module."

Case 20: Context Re-Hydration — Resuming a Paused Multi-Repo Story After Weeks Away

User Story: As a developer rotating across multiple projects, when I return to a project after weeks away, I want a single command that verifies dendrite pool links, summarizes completed vs. pending analysis chunks in dendrite store, restores my JetBrains IDE scopes, and briefs me on the exact next task.

Workflow:

dendrite story resume

Part VII: Infrastructure Mobility, Smart Backups & Federated Documentation (Cases 21–23)

Case 21: Zero-Data-Loss Service Migration (Local Workstation $\leftrightarrow$ Debian Server $\leftrightarrow$ Azure Cloud)

User Story: As an engineer who installed Dendrite on my local computer, I want to move heavy background services (dendrite-daemon, chunked analysis workers, dendrite store) to a Debian server, later migrate them to Azure Cloud, or repatriate them back from Azure to Debian/Local—with zero data loss and uninterrupted IDE integration.

5-Phase Atomic Migration Protocol

Pre-Flight Smart Backup: Automatically invokes dendrite backup run before state transfer.

WAL Journaling: Freezes store checkpoints and routes live JetBrains IDE story updates into a local Write-Ahead Log (.dendrite/wal.journal).

BLAKE3 Content-Addressed Sync: Transfers only missing dendrite store chunks and SQLite transaction pages over SSH/Tailscale (for Debian) or HTTPS/Blob API (for Azure).

Merkle Root Verification: Verifies that the target node's state digest matches the source node (source_blake3 == target_blake3).

Atomic Cutover & WAL Replay: Switches the active node endpoint in .dendrite/config.toml and replays any buffered WAL entries.

CLI Commands (Also Available in UI Compute Nodes Tab)

# 1. Move services from Local Workstation -> Debian Server

dendrite node bootstrap ssh://user@debian-server:22 --runtime systemd

dendrite node migrate --from local --to debian-server --verify-checksum

# 2. Move services from Debian Server -> Azure Cloud

dendrite node register azure-cloud --provider azure --resource-group "rg-dendrite" --storage "stdendrite"

dendrite node migrate --from debian-server --to azure-cloud --verify-checksum

# 3. Vice Versa: Repatriate from Azure Cloud -> Debian Server (or Local)

dendrite node migrate --from azure-cloud --to debian-server --decommission-source --verify-checksum

Case 22: Smart, Repo-Aware Backup & State Eviction Engine (dendrite backup)

User Story: As a user protecting my analysis store and multi-project stories, I want a smart backup routine that saves snapshots to a target directory, enforces a strict limit on versions (2 by default) and disk space, and automatically frees space when full by first removing old data that is already committed and persisted inside the Git repository.

How the Smart Repo-Aware Eviction Algorithm Works

When dendrite backup run triggers (manually, on schedule, or prior to node migration), it checks two boundaries:

Version Cap (max_versions = 2 by default): Retains at most two complete recovery points (slot-0 latest and slot-1 previous).

Space Quota (max_size_mb) & Git-Persisted Eviction Priority:

If the target backup directory reaches its space limit, Dendrite does not blindly delete unpersisted work. Instead, it runs a 3-priority eviction pass:

Priority 1 (Evict First — Git-Persisted Data): Cross-checks backup objects against git rev-list --objects --all across your pool repositories. Any source file, story specification, or ADR whose content hash is already committed and persisted in Git is stripped from the backup archive and replaced with a compact 40-byte Git SHA pointer (git://commit-sha/blob-sha).

Priority 2 (Evict Second — Recomputable Static AST Indexes): Evicts zero-token deterministic AST indexes that Go tree-sitter can regenerate in seconds.

Priority 3 (Protected — Never Evicted Automatically): Preserves high-cost LLM semantic summaries in dendrite store, uncommitted working-tree diffs, binary annotations, and active pool metadata.

# Configure Smart Backup (Defaults: max-versions=2, prune-git-persisted=true)

dendrite backup init --target-dir "/mnt/backups/dendrite" --max-versions 2 --max-size "2GB"

# Run smart backup now and inspect space saved via Git deduplication

dendrite backup run

dendrite backup status

Case 23: Multi-Project Documentation Federation & Live Docusaurus Serving (dendrite docs)

User Story: As a developer managing multiple repositories in a pool, I want to understand how Docusaurus handles multi-project documentation and use Dendrite to generate, aggregate, and serve unified documentation across all my projects.

How Docusaurus Works & How Dendrite Integrates It

How Docusaurus Works: Docusaurus is a static-site and Single-Page Application (SPA) generator powered by React and MDX (Markdown + embedded JSX components and Mermaid diagrams). In a standard single-project setup, a single @docusaurus/plugin-content-docs instance reads ./docs and compiles it into static HTML/JS bundles.

How Dendrite Federates Multiple Projects: For multiple repositories in dendrite pool, Dendrite generates a federated .dendrite/docs-hub/docusaurus.config.js that dynamically mounts each pool member's documentation (plus auto-generated story ADRs and dendrite pool Mermaid dependency graphs) using multi-instance docs plugins or virtual symlinks under /docs/<project-name>.

Built-In Serving Without Node.js Lock-in:

If Node.js/Bun is installed (e.g., when working in WebStorm), dendrite docs serve --live launches the hot-reloading Docusaurus dev server.

On lean environments or Debian servers without Node.js, dendrite docs serve compiles/renders the Markdown + Mermaid bundles and serves the documentation portal directly from Dendrite's embedded Go HTTP server (net/http + embedded static assets).

# Generate/update Markdown, API summaries, and Mermaid pool DAGs across all pool projects

dendrite docs sync

# Serve the unified multi-project documentation portal locally

dendrite docs serve --port 3030 --open

Part VIII: Dual-Mode Configuration (UI, CLI & nvim/vim) & Go CI/CD Publishing (Cases 24–25)

Case 24: Seamless Configuration via Optional UI, Ergonomic CLI, or nvim/vim

User Story: As a developer configuring Dendrite during or after installation, I want 100% feature parity between an optional graphical/terminal UI, fast CLI commands, and direct editing in nvim/vim with schema validation and section-jumping.

- Interactive UI Mode (Web UI & SSH TUI)

# Open the embedded localhost Web UI (127.0.0.1:7878) to manage Pool, Store, Token Policy, Backups, and Nodes

dendrite ui

# Open the interactive Terminal UI (Bubble Tea TUI) when connected to a headless Debian server over SSH

dendrite ui --tui

- Ergonomic CLI Configuration Commands

# View all configuration keys or filter by subsystem

dendrite config list

dendrite config get analysis.scheduler

# Modify configuration keys directly from the CLI with type & enum validation

dendrite config set \

ide.primary="goland" \

backup.max_versions=2 \

analysis.scheduler.min_quota_reserve_percent=20 \

analysis.scheduler.allow_zero_quota_override=false

- First-Class nvim / vim Workflow

Launch nvim or vim at a Specific Configuration Section:

# Opens .dendrite/config.toml in nvim (or vim) in a transactional staging buffer

dendrite config edit --editor nvim

# Jump cursor directly to the [analysis.scheduler] or [backup] table inside nvim:

dendrite config edit --section analysis.scheduler --editor nvim

(Executes nvim +/^\[analysis\.scheduler\] on a staged copy of .dendrite/config.toml).

Transactional Save-and-Validate Loop (:wq):

When you save and quit (:wq or ZZ), Dendrite validates the TOML syntax, file paths, and time-window formats before applying changes to the live daemon. If a validation error occurs on line 22, Dendrite prompts to re-open nvim +22 directly on the offending line.

Direct Manual nvim/vim Editing with LSP Autocompletion:

You can also open nvim .dendrite/config.toml directly at any time. Every generated config file includes a #:schema directive for the taplo TOML LSP in Neovim and a Vim modeline:

#:schema https://raw.githubusercontent.com/yasuomaidana/dendrite/main/schema/dendrite-config.schema.json

# vim: set ft=toml ts=2 sw=2 et:

[ide]

primary = "goland"                      # "rustrover" | "goland" | "webstorm" | "pycharm"

auto_sync_scopes = true

[pool]

auto_infer_dag = true

always_prompt_new_member_dir = true     # Always asks user for directory in CLI and UI

[store]

path = "~/.dendrite/store"

persist_until_manual_purge = true       # Analyze once; keep until user deletes via UI/CLI

chunk_size_tokens = 4000

[source_refs]

check_on_audit_reuse = true             # Always check stored source-refs on --audit-library-reuse

[analysis.scheduler]

cloud_provider = "gemini"

local_provider = "ollama"

mode = "policy"                         # "policy" | "always" (runs continuously until paused)

paused = false                          # Toggled via `dendrite analyze pause` or UI

night_window = "23:00-06:00"            # Local-only analysis window

weekly_reset = "SUN 23:59"

sprint_before_reset = "48h"             # Switch to Gemini paid tokens 2 days before weekly reset

min_quota_reserve_percent = 20          # Default safety reserve floor (20%)

allow_zero_quota_override = false       # Switch: set `true` to allow draining down to 0%

[backup]

enabled = true

target_dir = "/mnt/backups/dendrite"

max_versions = 2                        # Default: retain 2 versions

max_size_mb = 2048                      # Evicts Git-persisted blobs first when full

prune_git_persisted = true

[docs]

framework = "docusaurus"

port = 3030

Case 25: Multi-Channel Distribution (Brew, GitHub, Debian .deb, GHCR) & Automated Go CI/CD

User Story: As the maintainer of yasuomaidana/dendrite, I want an automated GitHub Actions + GoReleaser CI/CD pipeline that tests the Go codebase and publishes native binaries to GitHub Releases, updates a Homebrew Tap (brew), builds .deb packages for Debian servers, and pushes multi-arch Docker images to GHCR for Azure Cloud whenever a v*.*.* tag is pushed.

- .goreleaser.yaml (Root of Repository)

version: 2

project_name: dendrite

before:

hooks:

- go mod tidy

builds:

- id: dendrite

main: ./cmd/dendrite

binary: dendrite

env:

- CGO_ENABLED=0

goos:

- linux
- darwin
- windows

goarch:

- amd64
- arm64

ldflags:

- -s -w -X main.version={{.Version}} -X main.commit={{.Commit}}

archives:

- id: default

formats: [tar.gz]

format_overrides:

- goos: windows

formats: [zip]

brews:

- name: dendrite

repository:

owner: yasuomaidana

name: homebrew-tap

token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"

directory: Formula

homepage: "https://github.com/yasuomaidana/dendrite"

description: "Autonomous multi-project pool orchestrator, persistent binary/repo analyzer, and JetBrains IDE workflow engine"

install: |

bin.install "dendrite"

nfpms:

- id: debian-pkg

package_name: dendrite

vendor: yasuomaidana

homepage: "https://github.com/yasuomaidana/dendrite"

maintainer: "Yasuo Ignacio Maidana Pérez"

description: "Dendrite CLI and background daemon for Debian servers"

formats:

- deb

contents:

- src: ./deploy/systemd/dendrite-daemon.service

dst: /lib/systemd/system/dendrite-daemon.service

dockers:

- image_templates:
- "ghcr.io/yasuomaidana/dendrite:{{ .Version }}-amd64"
- "ghcr.io/yasuomaidana/dendrite:latest-amd64"

use: buildx

build_flag_templates:

- "--platform=linux/amd64"
- image_templates:
- "ghcr.io/yasuomaidana/dendrite:{{ .Version }}-arm64"
- "ghcr.io/yasuomaidana/dendrite:latest-arm64"

use: buildx

goarch: arm64

build_flag_templates:

- "--platform=linux/arm64"

- .github/workflows/ci-release.yml

```
name: CI & Release

on:
  push:
    branches: [main]
    tags: ['v*.*.*']
  pull_request:
    branches: [main]

permissions:
  contents: write
  packages: write

jobs:
  test:
    name: Test & Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
          cache: true
      - name: Run Tests & Coverage
        run: |
          go test -race -v -coverprofile=coverage.out ./...
          go tool cover -func=coverage.out
      - name: Run GolangCI-Lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest

  release:
    name: GoReleaser Release
    needs: test
    if: startsWith(github.ref, 'refs/tags/v')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
          cache: true
      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3
      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}

```

---

# Part III: Multi-Provider AI Execution, Semantic Hierarchical Git Governance & SOTA Project Lifecycler (Cases 21–32)

## Architectural Additions Overview

Part III refines and expands Dendrite as an enterprise-grade autonomous multi-project orchestrator with:

- **Multi-Provider AI Execution**: Native direct Ollama streaming (`/api/chat`), bridged local inference, GitHub Copilot CLI/API, and GitHub CLI wrappers (`gh`).
- **5-Tier Git Version Governance**: Strict deterministic interception of every `git` and `gh` command validating capabilities across Levels 1–5 (`conservative`, `super-safe`, `safe`, `normal`, `yolo`).
- **Semantic Hierarchical Branching & Merging**: Strict 3-level tree structure (`milestone/<id>` $	o$ `story/<id>` $	o$ `feature/<id>`), ensuring merges occur level-by-level without bypassing parents or polluting `main`.
- **Relational DAG Ingestion & Next Story Prioritizer**: `dendrite stories next` calculating critical path unblock weight across Plane, GitHub, and local cache.
- **Bidirectional Penpot UI Auto-Documentation**: Deep-linking component states and token diffs between code, Penpot frames, and Plane tickets.
- **Dual-Source Self-Update & Dev Mode**: In-place atomic binary swaps with rollback safeguards, defaulting to local Go source compilation in `--dev` mode.
- **Polyglot Environment Bootstrapper & Canonical Design-Pattern Analyzer**: Stack fingerprinter generating `.agents/`, `.ai/`, `.gemini/`, and `AGENTS.md` with complete  Canonical Design Patterns catalogs and SOTA updates.
- **Offline-First Local Cache (Turso/libSQL)**: Embedded WAL database with transactional outbox queue and automated `.gitignore` protection.
- **Zero-Leak Secret Interception & Quality Verification**: Pre-commit entropy scanners and pre-PR automated test harnesses.
- **Resilience & Observability**: Quota-aware mid-task Copilot-to-Ollama failover, ephemeral Git worktrees, and immutable audit logging (`dendrite audit`).

---

### Case 21: Multi-Provider AI Auto-Negotiation with Direct Native Ollama Fallback (`dendrite agent run`)

**User Story**: As a developer running local AI models on Apple Silicon or workstation hardware alongside GitHub Copilot, I want Dendrite to probe my local environment, auto-negotiate between Copilot and native Ollama streaming HTTP API, and allow seamless execution without requiring external wrapper scripts.

**Target IDEs**: GoLand, RustRover, WebStorm, PyCharm.

**Workflow**:

```
# Probe environment and launch agent for active story
dendrite agent run --story DEN-051 --provider auto

# Output:
# [DISCOVERY] GitHub CLI (gh) authenticated (user: yasuomaidana)
# [DISCOVERY] Ollama active at http://127.0.0.1:11434 (models: qwen2.5-coder:14b, llama3.2:3b)
# [RUNTIME] Selected Provider: native-ollama (qwen2.5-coder:14b) [latency: 12ms]

```

**JetBrains Workflow**: External tools watcher catches local file modifications in real time and reloads symbol indexes.

---

### Case 22: Level 1 (`conservative`) Human-in-the-Loop Diff Review & Patch Export

**User Story**: As a security-conscious engineer working on mission-critical banking code, I want the agent to operate in `conservative` mode, generating changes only as unstaged diffs or patch files while strictly blocking all commits, branches, and PR creation.

**Workflow**:

```
dendrite agent run --story PROJ-101 --autonomy conservative
# Agent attempts: git commit -m "feat: implement payment gateway"
# [POLICY REJECTED] Autonomy Level 1 (conservative) blocks git commit.
# [ACTION] Changes staged to .dendrite/patches/PROJ-101.patch for human review.

```

---

### Case 23: Level 2 (`super-safe`) Automated Agent Branch Isolation

**User Story**: As an engineering lead, I want agents to work in isolated branches (`agent/*` or `story/*`), creating local commits without having permission to merge branches or open pull requests.

**Workflow**:

```
dendrite agent run --story PROJ-101 --autonomy super-safe
# [GIT GOV] Created and switched to branch: agent/PROJ-101
# [GIT GOV] Committed changes: 4 files changed, +182 insertions
# [GIT GOV] Attempted gh pr create -> BLOCKED by Level 2 (super-safe) policy.

```

---

### Case 24: Level 3 (`safe`) Strict 1:1 Story-to-PR Gating with Helper Sub-Branches

**User Story**: As a scrum master, I want agents working on a story to create sub-feature branches, merge them into their parent story branch, and open exactly one PR against `main` (with auto-merge blocked).

**Workflow**:

```
dendrite agent run --story PROJ-102 --autonomy safe
# [GIT GOV] Merged feature helper sub-branches into story/PROJ-102
# [GIT GOV] gh pr create --base main --head story/PROJ-102 [ALLOWED]
# [GIT GOV] gh pr merge -> BLOCKED by Level 3 policy (requires human reviewer approval).

```

---

### Case 25: Level 4 (`normal`) Semantic Hierarchical Multi-Story Consolidation

**User Story**: As a release engineer managing a milestone with multiple co-dependent stories, I want Dendrite to enforce a 3-tier hierarchy (`milestone/<id>` $	o$ `story/<id>` $	o$ `feature/<id>`), merge sub-tasks level-by-level, and submit a single consolidated PR for the entire milestone.

**Workflow**:

```
dendrite agent run --milestone M1-Auth --autonomy normal
# 1. Scaffolds milestone branch: milestone/M1-Auth
# 2. Scaffolds story branches: story/PROJ-104 and story/PROJ-105
# 3. Level 1 Merge: feature/* merged into story/PROJ-104 and story/PROJ-105
# 4. Level 2 Merge: story/PROJ-104 and story/PROJ-105 merged into milestone/M1-Auth
# 5. Level 3 Consolidated PR: Opens PR milestone/M1-Auth -> main (merge into main remains blocked for human sign-off).

```

---

### Case 26: Relational Ticket-Milestone DAG Ingestion & Context-Aware Active Story Loading (`dendrite stories next`)

**User Story**: As a developer starting my workday, I want Dendrite to analyze all open tickets and milestones across Plane and local cache, calculate critical-path unblock weights, and automatically check out the highest-priority story in my IDE.

**Workflow**:

```
dendrite stories link --analyze
# [DAG SOLVER] Ingested 24 tickets, 4 milestones. Detected 0 circular dependencies.

dendrite stories next
# [PRIORITY] Selected DEN-051 (Unblock Weight: 26.5 SP - blocks DEN-052, DEN-053).
# [GIT GOV] Checked out branch: story/DEN-051 off milestone/M24-pool-dag.
# [IDE HOOK] Injected .idea/scopes/Dendrite_Active_Story.xml into GoLand.

```

---

### Case 27: Bidirectional Penpot UI Auto-Documentation & Plane Deep-Link Annotation (`dendrite ui`)

**User Story**: As a frontend developer updating Svelte components, I want Dendrite to automatically detect design token changes, create component specification frames in Penpot, and post deep-links into the Plane ticket.

**Workflow**:

```
# Agent updates Button.svelte and tailwind.config.ts
dendrite ui doc-sync --story UI-204
# [PENPOT MCP] Synthesized component spec frame: "Button.svelte - Story UI-204"
# [PLANE SYNC] Posted comment to Plane issue UI-204 with direct Penpot frame deep-link.

```

---

### Case 28: Self-Update Lifecycle (`dendrite update`) with Checksum Verification & Rollback Safeguard

**User Story**: As a system administrator, I want to update Dendrite with a single command, automatically verifying SHA-256 signatures and safely rolling back to the previous binary if an update fails validation.

**Workflow**:

```
dendrite update
# [RELEASE] Found latest version v1.4.0 (active: v1.3.0)
# [DOWNLOAD] dendrite_darwin_arm64.tar.gz
# [VERIFY] SHA-256 checksum verified: 9b2d... [MATCH]
# [SWAP] Atomic in-place binary swap completed successfully.

```

---

### Case 29: Developer Mode (`dendrite update --dev`) Dual-Source Engine (Local Source Default & Remote Override)

**User Story**: As a contributor modifying Dendrite source code, I want `dendrite update --dev` to build and install from my local source tree by default, and allow testing remote release candidates via `--source remote`.

**Workflow**:

```
# Compile and install from local checked-out tree:
dendrite update --dev

# Or explicitly test remote candidate in dev mode:
dendrite update --dev --source remote

```

---

### Case 30: Polyglot Environment Bootstrapper &  Canonical Design-Pattern Analyzer (`dendrite env prepare/update --sota`)

**User Story**: As an architect bootstrapping a new Go + Svelte project, I want Dendrite to replace legacy Rust agent instructions with idiomatic Go and Svelte rules, seed the  canonical design-pattern catalog, and provide `--sota` updates.

**Workflow**:

```
# Bootstrap stack-specific agent environment
dendrite env prepare

# Update with state-of-the-art good practices
dendrite env update --sota
# [SOTA UPDATE] Updated .agents/rules.md, AGENTS.md, and .gemini/skills/ with 2026 Go concurrency idioms.

```

---

### Case 31: Offline-First Local Cache & Outbox Sync Engine with Turso/libSQL & Automated `.gitignore` Hygiene

**User Story**: As a developer traveling on a train without internet access, I want Dendrite to seamlessly serve tickets from local embedded Turso cache, record offline completions in an outbox table, replay them upon reconnection, and ensure cache files are excluded from git.

**Workflow**:

```
# Offline execution:
dendrite stories next
# [OFFLINE CACHE] Serving tickets from .dendrite/cache/dendrite.db (WAL mode)
# [OUTBOX] Recorded completion of UI-101 into outbox_mutations table.

# When connectivity returns:
# [OUTBOX REPLAY] Synchronized 1 mutation to Plane v2 with HTTP 200 OK.

```

---

### Case 32: Enterprise Multi-Agent Resilience: Secret Interception, Quality Verification Gate, Quota Failover, Worktree Isolation & Immutable Audit Logging (`dendrite audit`)

**User Story**: As an enterprise team lead, I want end-to-end multi-agent resilience: intercepting secrets before commits, enforcing test gates before PRs, failing over to local Ollama on quota limits, isolating tasks in Git worktrees, and maintaining an immutable audit log.

**Workflow**:

```
# 1. Ephemeral worktree execution:
dendrite agent run --story PROJ-104 --worktree --autonomy normal

# 2. Pre-Commit Secret Scanner Intercepts Token:
# [SECRET INTERCEPTOR] Staged diff contains high-entropy string (pattern: GitHub PAT).
# [ABORT] Commit aborted. Secret redacted from outbound prompt context.

# 3. Quota Failover:
# [COPILOT] HTTP 429 Too Many Requests received.
# [FAILOVER] Checkpointed state in Turso. Routing inference to local Ollama (qwen2.5-coder:14b).

# 4. Mandatory Test Gate Before PR:
# [QUALITY GATE] Running verification suite: go test ./... && golangci-lint run
# [PASSED] Exit code 0. gh pr create permitted.

# 5. Query Full Audit Log:
dendrite audit --story PROJ-104
# [AUDIT TRACE]
# Timestamp            Autonomy   Action       Details
# 2026-10-14 20:45:01  normal     GIT_BRANCH   Created story/PROJ-104 off milestone/M1
# 2026-10-14 20:45:10  normal     SECRET_CHECK Clean diff verified
# 2026-10-14 20:45:12  normal     GIT_COMMIT   Committed 3 files (+120 lines)
# 2026-10-14 20:46:00  normal     FAILOVER     Copilot 429 -> Ollama failover successful
# 2026-10-14 20:47:15  normal     QUALITY_GATE Test & lint passed (exit 0)
# 2026-10-14 20:47:20  normal     GH_PR_CREATE Opened PR #42 (milestone/M1 -> main)

```

---

### Case 33: Multi-IDE Ecosystem Support & Decoupled Architecture (`FEAT-100` / `TKT-IDE-001` - `003`)

**User Story**: As an engineering team using diverse editors, I want VS Code supported at Normal priority with a native extension (`TKT-IDE-002`) and a decoupled shared JSON-RPC core client (`TKT-IDE-001`), while keeping the broader multi-IDE expansion (`TKT-IDE-003`: Neovim, Zed, IntelliJ) at Lowest priority so it does not delay sprint milestones.

**Target IDEs**: VS Code, Neovim, Zed, IntelliJ Platform, Cursor, Windsurf.

**Workflow**:

```
# 1. VS Code Extension connects to local Dendrite daemon via shared JSON-RPC engine:
# [RPC BRIDGE] Connected to unix://~/.dendrite/dendrite.sock (TKT-IDE-001)
# [VS CODE EXT] Activated status bar indicator (Tier: Local Ollama | Cache: ONLINE)
# [WEBVIEW] Rendered interactive Pool DAG in VS Code primary sidebar.

# 2. Neovim Telescope Pool Picker (TKT-IDE-003):
# In Neovim: :Telescope dendrite pool
# [NVIM] Queried daemon via stdio JSON-RPC; displays pool members and allows toggling published/branch/local/fork.

```

---

### Case 34: Automated Stories / Tickets / Milestones DAG Connector (`FEAT-DAG-CONNECTOR`)

**User Story**: As an architect writing a technical design prompt or adding new tickets, I want Dendrite to automatically trigger the Stories / Tickets / Milestones Connector on `on_design_prompt_write` and `on_ticket_insert`, extract graph edges, elevate blocker priorities from `Lowest` to `Normal` via bottom-up inheritance, update the Turso cache and Plane, and emit a sync manifest table.

**Workflow**:

```
# 1. Insert new ticket requiring shared IDE bridge:
dendrite dev ticket set TKT-IDE-002 "VS Code Extension" --priority Normal --depends-on TKT-IDE-001

# 2. Synchronous DAG Connector Reconcile Hook triggers:
# [DAG CONNECTOR] Hook triggered: on_ticket_insert (TKT-IDE-002)
# [DAG CONNECTOR] Evaluated bottom-up priority inheritance:
#   - TKT-IDE-001: Assigned=Lowest -> Effective=Normal (Elevated by dependent TKT-IDE-002)
#   - TKT-IDE-003: Assigned=Lowest -> Effective=Lowest (No high-priority dependents)
# [TURSO CACHE] Updated DAG edges in .dendrite/cache/dendrite.db
# [PLANE SYNC] Pushed updated effective priorities to Plane v2 (:8090)

# 3. Connector Sync Summary Output:
# +-------------+-------------------+-------------------+--------------------+------------------+-------------------+
# | Entity ID   | Type              | Assigned Priority | Effective Priority | Linked Milestone | DAG Edges Updated |
# +-------------+-------------------+-------------------+--------------------+------------------+-------------------+
# | TKT-IDE-001 | Shared Core Engine| Lowest            | Normal             | M1 - VS Code MVP | blocks:TKT-IDE-002|
# | TKT-IDE-002 | VS Code Extension | Normal            | Normal             | M1 - VS Code MVP | depends:TKT-IDE-01|
# | TKT-IDE-003 | Extended IDEs     | Lowest            | Lowest             | M3 - Backlog     | none              |
# +-------------+-------------------+-------------------+--------------------+------------------+-------------------+

```
