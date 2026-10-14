# Multi-Project Workspace Isolation & State Management

# Multi-Project Workspace Isolation & State Management

# FEAT-005: Multi-Project Workspace Isolation & State Management

## 1. User Story & Business Objective

As a lead engineer juggling multiple client repositories and internal codebases, I need Dendrite to manage and switch between distinct project workspaces, ensuring that AST databases, token spend budgets, virtual keys, Docker volumes, and local MCP state remain strictly isolated per project without cross-contamination.

---

## 2. Technical Boundaries & Specifications

1. **Workspace Hierarchy**:

- Root registry stored in `~/.dendrite/config.yaml` or project-local `.dendrite/workspace.yaml`.
- Each project maintains an isolated data directory:
- `~/.dendrite/projects/<project-name>/graphify.db` (Isolated AST knowledge graph).
- `~/.dendrite/projects/<project-name>/spend.json` (Local spend cache and virtual key mapping).
- `~/.dendrite/projects/<project-name>/scratch/` (Temporary binary artifacts).

2. **Virtual Key Segregation**:

- When switching projects, the CLI binds requests to that project's LiteLLM virtual key (`key_alias: "project-<name>"`), preventing project A from exhausting project B's financial quota.

3. **Typestate Pattern in CLI**:

- Workspaces enforce deterministic state transitions in code:

`Workspace<Unregistered>` $\to$ `Workspace<Registered>` $\to$ `Workspace<Active>`.

stateDiagram-v2

[*] --&gt; Unregistered

Unregistered --&gt; Registered: dendrite project add &lt;name&gt; &lt;path&gt;

Registered --&gt; Active: dendrite project switch &lt;name&gt;

Active --&gt; Suspended: dendrite project switch &lt;other&gt;

Suspended --&gt; Active: dendrite project switch &lt;name&gt;

Active --&gt; [*]: dendrite project remove &lt;name&gt;</code></pre>

---

## 3. Acceptance Criteria & Verification Matrix

- [x] **AC-1**: `dendrite project switch <name>` updates the active context in $< 50\text{ms}$ and points all local MCP servers to the selected project's graph database.
- [x] **AC-2**: Token spend and usage statistics are tracked independently per project key in the LiteLLM PostgreSQL database.
- [x] **AC-3**: Git repository operations and branch creation in one workspace have zero visibility or side-effects on other workspaces.

---

## 4. Definition of Done (DoD) & Persistence Verification

- **Automated Tests**: Unit and integration tests verify workspace registration, switching, concurrency locking, and removal.
- **Persistence Verification**: Workspace registry and spend cache survive CLI reinstallation and system reboot.
- **Complexity Guardrails**: Workspace management structs avoid God-struct anti-patterns; cognitive complexity of context switching is $\le 10$.
