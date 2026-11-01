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
