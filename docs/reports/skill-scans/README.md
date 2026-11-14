# Skill pre-scans (DEN-424)

SkillSpector (static, `--no-llm`, `--recursive`) over the whole upstream repos, before any install.

| Repo | Commit | Score | Issues |
|---|---|---|---|
| google/elephant-goldfish-model | 403d8881 | 100 CRITICAL | 117 |
| marconae/speq-skill | 02530fad | 100 CRITICAL | 86 |
| obra/superpowers | 2ce8aa15 | 100 CRITICAL | 193 |

Dominant categories: Agent Snooping, Prompt Injection (templates), Tool Misuse, Excessive Agency, Rogue Agent. Whole-repo scans include docs, issue templates and tests, so the score is inflated. Policy: >50 is not installed without a recorded human decision.

## Scoped re-scan (installable dirs only, static)

| Target | Score | Verdict |
|---|---|---|
| EGM skills (7) | max 30 (egm-draft) | installed (claude + agy) |
| speq skills (20) | max 26 (speq-plan) | held with the installer |
| speq `install.sh` | 60 HIGH | LLM pass (ollama qwen3:4b, 3/4 calls) unchanged at 60 |
| superpowers skills (15) | brainstorming 52, systematic-debugging 77, writing-skills 100; rest <= 49 | not installed |
| superpowers hooks | 10 LOW | n/a |

`install.sh` findings read by hand: TM2/SC2 hit a `curl | bash` usage comment and echoed help text; E1 is the Serena prompt (asks before installing). Real residual risk: the release binary is downloaded without a checksum check.

Per-skill JSON under `static-scoped/`; the LLM report under `llm-qwen3-4b/`. Policy: >50 needs a recorded human decision (pending for speq and for superpowers).

## Per-skill scores on record (static, SkillSpector, scoped scans)

| Source | Skill | Score | Severity |
|---|---|---|---|
| egm-skills | egm-bootstrap | 18 | LOW |
| egm-skills | egm-brainstorm | 0 | LOW |
| egm-skills | egm-draft | 30 | MEDIUM |
| egm-skills | egm-drift-report | 0 | LOW |
| egm-skills | egm-resolve | 0 | LOW |
| egm-skills | egm-review | 0 | LOW |
| egm-skills | egm-review-loop | 0 | LOW |
| speq-skills | speq-adr-rules | 0 | LOW |
| speq-skills | speq-audit | 9 | LOW |
| speq-skills | speq-cli | 0 | LOW |
| speq-skills | speq-code-guardrails | 0 | LOW |
| speq-skills | speq-code-review | 0 | LOW |
| speq-skills | speq-code-tools | 21 | MEDIUM |
| speq-skills | speq-design-philosophy | 21 | MEDIUM |
| speq-skills | speq-ext-library | 0 | LOW |
| speq-skills | speq-git-discipline | 0 | LOW |
| speq-skills | speq-git-operations | 0 | LOW |
| speq-skills | speq-implement | 9 | LOW |
| speq-skills | speq-implement-pr | 9 | LOW |
| speq-skills | speq-mission | 7 | LOW |
| speq-skills | speq-plan | 26 | MEDIUM |
| speq-skills | speq-plan-pr | 9 | LOW |
| speq-skills | speq-plan-review | 0 | LOW |
| speq-skills | speq-planning | 0 | LOW |
| speq-skills | speq-record | 16 | LOW |
| speq-skills | speq-spec-merge | 0 | LOW |
| speq-skills | speq-writing-guardrails | 0 | LOW |
| superpowers-skills | brainstorming | 52 | HIGH |
| superpowers-skills | diagnosing-superpowers | 26 | MEDIUM |
| superpowers-skills | dispatching-parallel-agents | 0 | LOW |
| superpowers-skills | executing-plans | 17 | LOW |
| superpowers-skills | finishing-a-development-branch | 17 | LOW |
| superpowers-skills | receiving-code-review | 0 | LOW |
| superpowers-skills | requesting-code-review | 7 | LOW |
| superpowers-skills | subagent-driven-development | 17 | LOW |
| superpowers-skills | systematic-debugging | 77 | HIGH |
| superpowers-skills | test-driven-development | 0 | LOW |
| superpowers-skills | using-git-worktrees | 11 | LOW |
| superpowers-skills | using-superpowers | 49 | MEDIUM |
| superpowers-skills | verification-before-completion | 7 | LOW |
| superpowers-skills | writing-plans | 0 | LOW |
| superpowers-skills | writing-skills | 100 | CRITICAL |

Installed over the 50 threshold by recorded human decision (DEN-424 decisions): superpowers brainstorming 52, systematic-debugging 77, writing-skills 100.

## Smoke runs (2026-11-01)

- `agy -p "Reply with exactly: ok" --model gemini-3.8-flash-low --output-format json` -> `{"status":"SUCCESS","response":"ok"}`
- `agy -p "/egm:egm-brainstorm ..."` (no writes) -> status SUCCESS; reports artifact directory `docs/specs` from AGENTS.md.
- `claude -p "/egm:egm-brainstorm ..."` -> skill and its lenses listed.
