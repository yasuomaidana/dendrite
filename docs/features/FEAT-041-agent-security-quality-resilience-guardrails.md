---
id: FEAT-041
title: "Autonomous Agent Security, Quality, Resilience & Audit Guardrails (Epics 8–12)"
status: approved
plane_epic: PROJ-E-16
dependencies: [FEAT-013, FEAT-035, FEAT-040]
repository: dendrite
tags: [security-guardrail, pre-commit-secrets, test-gate, copilot-failover, git-worktree, immutable-audit-log]
---

# FEAT-041: Autonomous Agent Security, Quality, Resilience & Audit Guardrails (Epics 8–12)

## 1. Executive Summary & Problem Statement

### The Problem
When autonomous agents are granted high autonomy levels (Level 2 `super-safe` through Level 5 `yolo`), five critical operational hazards emerge:
1. **Secret Leakage**: Agents can accidentally stage and commit `.env` files, API keys (Tailscale, Plane, Penpot, GitHub PATs), or test credentials into Git repositories.
2. **Broken PRs & Broken Main**: Agents opening PRs or merging directly (Level 5 `yolo`) can merge broken builds or failing tests if no deterministic verification harness blocks them.
3. **Mid-Task Provider Outage**: If GitHub Copilot hits rate limits (`429 Too Many Requests`) or internet connectivity drops mid-task, agents crash and leave orphaned, incomplete branches.
4. **Working Tree Collisions**: When agents switch branches or run builds in the developer's main working tree, uncommitted developer work is disrupted or corrupted by merge conflicts.
5. **Lack of Auditability**: Autonomous operations execute dozens of CLI commands without an immutable, auditable local ledger explaining *why* an action was taken or *which* model generated a given diff.

### User Story & Architectural Pillars
As an enterprise engineering director and security auditor, I need Dendrite to enforce five foundational guardrails:
1. **Pre-Commit Secret Interception (Epic 8)**: Intercept every `git commit` in Levels 2–5 and abort if sensitive keys or high-entropy tokens are detected in the diff.
2. **Pre-PR / Pre-Merge Test & Lint Gate (Epic 9)**: Block `gh pr create` and `gh pr merge` unless the project verification suite passes with exit code `0`.
3. **Hybrid Quota Failover (Epic 10)**: Seamlessly checkpoint and fail over from cloud Copilot to local Ollama on HTTP 429 or network disconnect.
4. **Git Worktree Isolation & Rollback (Epic 11)**: Mount isolated worktrees (`dendrite-work/<repo>-<story-id>`, created by `dendrite work new`) for Level 2–5 agents and safely abort and roll back failed merges.
5. **Immutable Agent Audit Log (Epic 12)**: Persist an append-only cryptographic event trace in Turso queryable via `dendrite audit --story <id>`.

```mermaid
flowchart TD
    subgraph "Autonomous Agent Action Execution"
        A["Agent Attempts Git / Tool Action<br/>(commit, pr create, pr merge)"] --> SG{"Epic 8: Secret Scanner Guardrail"}
    end

    SG -->|Secret Detected| AbortSec["Abort Commit immediately<br/>Strip secrets from context & alert developer"]
    SG -->|Diff Clean| TG{"Epic 9: Pre-PR / Merge Test Gate"}

    TG -->|Tests Fail (exit != 0)| BlockPR["Block PR / Merge<br/>Agent self-heals or requests human review"]
    TG -->|Tests Pass (exit == 0)| WT{"Epic 11: Worktree Mount"}

    WT --> IsoTree["Execute in isolated worktree:<br/>dendrite-work/<repo>-<story-id>"]
    IsoTree --> Fallback{"Epic 10: Provider Failover"}

    Fallback -->|Copilot HTTP 429 / Offline| OllamaNative["Checkpoint & Route to Local Ollama"]
    Fallback -->|Provider Healthy| CompleteAction["Complete Story Work"]

    CompleteAction --> Audit["Epic 12: Write to Immutable Turso Audit Log<br/>(Command, Autonomy Level, Model, SHA, Timestamp)"]
```

---

## 2. Technical Architecture & Idiomatic Design Patterns

### 2.1 Epic 8: Pre-Commit Secret Scanner Guardrail
The process interceptor parses staged diffs (`git diff --cached`) using regular expressions and Shannon entropy calculations:
- **Patterns Scanned**: GitHub PATs (`ghp_*`), OpenAI/Anthropic/Google keys, AWS access keys, SSH private keys, JWTs, Tailscale keys, Plane/Penpot tokens, `.env` file additions.
- **Shannon Entropy**: Entropy $H > 4.5$ on strings $> 20$ chars triggers secret review.
- **Fail-Safe**: If a secret is matched, the command returns exit code `126`, un-stages the file, and scrubs the value from outbound LLM prompt buffers.

### 2.2 Epic 9: Pre-PR & Pre-Merge Verification Harness
Before executing `gh pr create` or `gh pr merge`:
1. Dendrite inspects `AGENTS.md` or project config for verification commands (e.g. `go test -race ./...`, `golangci-lint run`, `npm run test`).
2. Runs the suite in an isolated subprocess.
3. If tests fail, Dendrite prohibits PR creation or merge, enters self-healing loop ($\le 3$ iterations), and downgrades to human review if unresolved.

### 2.3 Epic 10: Hybrid Quota Scheduler & Automatic Provider Failover
Bridges cloud Copilot and local Ollama Metal:
- If a Copilot request returns `429 Too Many Requests` or `ECONNREFUSED`, Dendrite commits the current agent step state to `.dendrite/cache/dendrite.db`.
- Automatically switches active inference provider to `ollama:<default_model>` (`127.0.0.1:11434`) and resumes the story on the local branch without losing prompt context.

### 2.4 Epic 11: Ephemeral Git Worktree Isolation & Merge Safe-Stop
- **Worktree Isolation**: `dendrite work new <story-id>` (a `git worktree add` into `dendrite-work/<repo>-<story-id>` on branch `agent/<story-id>`) keeps the developer's main IDE working tree untouched while agents run tests and modify code.
- **Deterministic Merge Safe-Stop**: If `git merge` encounters conflicts during branch consolidation (Level 4/5) and automated 3-way AST resolution fails tests:
  $$\text{Invoke } \texttt{git merge --abort} \implies \text{Restore pre-merge ref from Turso} \implies \text{Flag for human review}$$

### 2.5 Epic 12: Immutable Agent Audit Log (`dendrite audit`)
Stored in Turso table `agent_audit_log`:
```sql
CREATE TABLE IF NOT EXISTS agent_audit_log (
    event_id TEXT PRIMARY KEY,
    story_id TEXT NOT NULL,
    autonomy_level TEXT NOT NULL,
    provider_used TEXT NOT NULL,
    model_identifier TEXT NOT NULL,
    command_executed TEXT NOT NULL,
    policy_verdict TEXT NOT NULL,
    diff_hash_blake3 TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

---

## 3. Behavioral Scenarios (Gherkin)

### Scenario 8A: Autonomous Commit Secret Interception
- **Given** an agent running in Level 2 (`super-safe`) through Level 5 (`yolo`) attempts to stage or commit files
- **When** the pre-commit interceptor scans the staged diff and detects high-entropy strings or known token patterns (GitHub PATs, Tailscale keys, Plane/Penpot tokens, private keys)
- **Then** Dendrite immediately aborts the commit, strips the sensitive value from any outbound LLM prompt context, and alerts the user to move the credential into environment variables or secret stores.

### Scenario 9A: Mandatory Test & Lint Gate Before PR/Merge
- **Given** an agent in Level 3, 4, or 5 finishes coding a story and attempts `gh pr create` or `gh pr merge`
- **When** Dendrite executes the project's verification suite defined in `AGENTS.md` (e.g., `go test ./...`, `golangci-lint`, `npm run check`)
- **Then** the PR creation or merge is only permitted if the exit code is `0`; if tests fail, the agent remains on the feature branch to self-heal the failure or downgrades to human review.

### Scenario 10A: Mid-Task Failover from Copilot to Local Ollama
- **Given** an agent is executing a task using GitHub Copilot and encounters an HTTP `429 Too Many Requests` or network disconnect
- **When** local Ollama is available on the host
- **Then** Dendrite checkpoints the current step in the Turso cache and seamlessly routes subsequent inference calls to the local Ollama model to complete the local branch work.

### Scenario 11A: Ephemeral Worktree Execution (`dendrite agent --worktree`)
- **Given** the developer has uncommitted local edits in their main working directory
- **When** an autonomous agent starts a Level 2–5 task
- **Then** Dendrite mounts an isolated git worktree under `dendrite-work/<repo>-<story-id>` so the agent can branch, build, and test without touching the developer's active IDE workspace.

### Scenario 11B: Merge Conflict Safe-Stop & Checkpoint Rollback
- **Given** an agent in Level 3, 4, or 5 encounters a non-trivial merge conflict during branch consolidation
- **When** automated AST/3-way conflict resolution fails the build gate
- **Then** Dendrite aborts the merge (`git merge --abort`), restores the pre-merge branch ref from the Turso state log, and flags the story for human conflict resolution.

### Scenario 12A: Tracing Autonomous Actions
- **Given** an agent executed branch merges and PR creation under Level 4 (`normal`)
- **When** the developer runs `dendrite audit --story PROJ-104`
- **Then** Dendrite queries the local Turso audit table and outputs a chronological trace of every Git command, policy check verdict, LLM provider used, and linked Penpot/Plane sync event.

---

## 4. Acceptance Criteria & Definition of Done

- [ ] **AC-1 (Pre-Commit Secret Guardrail)**: Scans staged diffs for high-entropy tokens and known keys, blocking commits and stripping credentials from LLM prompt buffers.
- [ ] **AC-2 (Pre-PR/Merge Quality Gate)**: Verifies build, lint, and test suites with exit code `0` before permitting PR creation or merges.
- [ ] **AC-3 (Mid-Task Provider Failover)**: Checkpoints state and fails over transparently from cloud Copilot to local Ollama on HTTP 429 or disconnect.
- [ ] **AC-4 (Worktree Isolation & Safe Rollback)**: Runs tasks in `dendrite-work/<repo>-<id>` and executes `git merge --abort` with Turso state rollback on conflict failures.
- [ ] **AC-5 (Immutable Audit Ledger)**: Implements `dendrite audit --story <id>` querying append-only cryptographic event traces in Turso.
- [ ] **AC-6 (DoD & Complexity Thresholds)**: Validated via Testcontainers integration suites; cyclomatic complexity $\le 15$; cognitive complexity $\le 20$; test coverage $\ge 85\%$.
