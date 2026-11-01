# Skill pre-scans (DEN-424)

SkillSpector (static, `--no-llm`, `--recursive`) over the whole upstream repos, before any install.

| Repo | Commit | Score | Issues |
|---|---|---|---|
| google/elephant-goldfish-model | 403d8881 | 100 CRITICAL | 117 |
| marconae/speq-skill | 02530fad | 100 CRITICAL | 86 |
| obra/superpowers | 2ce8aa15 | 100 CRITICAL | 193 |

Dominant categories: Agent Snooping, Prompt Injection (templates), Tool Misuse, Excessive Agency, Rogue Agent. Whole-repo scans include docs, issue templates and tests, so the score is inflated. Policy: >50 is not installed without a recorded human decision.
