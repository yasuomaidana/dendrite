---
id: FEAT-002
title: Token Budget Governance, Rate Limiting & Model Fallback Cascade
status: approved
plane_epic: PROJ-E-02
dependencies: [FEAT-001]
repository: dendrite
tags: [governance, litellm, postgres, redis, ai-models]
---

# FEAT-002: Token Budget Governance, Rate Limiting & Model Fallback Cascade

## 1. User Story & Business Objective
As a project lead, I need real-time token spend tracking, hierarchical budget ceilings, and automatic fallback from expensive cloud models to zero-cost local Ollama inference, so that autonomous agent execution never exceeds pre-allocated budgets or halts unexpectedly due to provider rate limits.

---

## 2. Technical Boundaries & Specifications

1. **LiteLLM Gateway Deployment**:
   - Proxy port: `4000`.
   - Backed by PostgreSQL 16 (spend logs, virtual key metadata) and Redis 7 (rate limiting, sliding window counters).
2. **Virtual Keys & Sliding Windows**:
   - Hourly (`1h`), Daily (`24h`), Weekly (`7d`), and Monthly (`30d`) windows.
   - Separate keys for `interactive-ide` and `autonomous-coding-agent`.
3. **Model Fallback Chain**:
   - `gemini-2.5-pro` $\to$ `copilot-sonnet` $\to$ `gemini-2.5-flash` $\to$ `local-qwen-coder`.
   - Context escalation: When prompt $> 32\text{k}$ tokens on `local-qwen-coder`, escalate directly to `gemini-2.5-pro`.
4. **Supervisory Governance Modes**:
   - Exhaustion-capped batch mode.
   - Hard financial capping mode.
   - Rate-bounded throttling mode: $R_{\text{target}} = R_{\text{consumed}} + (0.30 \times R_{\text{weekly\_limit}})$.
   - Time-bounded curfew (`07:00 AM`) & SoC thermal ceiling ($85^\circ\text{C}$).

---

## 3. Acceptance Criteria & Verification Matrix

- [x] **AC-1**: Virtual keys enforce sliding budget limits; requests exceeding limits receive seamless redirects to `local-qwen-coder` rather than client errors.
- [x] **AC-2**: Zero-cost model `local-qwen-coder` ($0.00/token) is never rejected by spend checks (`skip_budget_checks: true`).
- [x] **AC-3**: Upstream Copilot requests preserve `X-Initiator` headers and drop unsupported client parameters.
- [x] **AC-4**: Supervisor monitors SoC die temperature and suspends tasks if temperature exceeds $85^\circ\text{C}$ on Apple Silicon.

---

## 4. Definition of Done (DoD) & Persistence Verification

- **Automated Tests**: Unit tests simulate provider 429 and spend-cap responses, asserting that client requests succeed via local fallback.
- **Persistent Layer Verification**: Spend log records and key budget updates in PostgreSQL verified against real database instances spun up via Testcontainers.
- **Coverage**: Business logic branch coverage $\ge 90\%$.
