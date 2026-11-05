---
id: complexity-second-opinion
title: "Complexity: the second opinion in CI"
sidebar_label: "Complexity cross-check"
---

# Complexity: the second opinion in CI

Function complexity is checked twice.

1. **Dendrite's own check.** `dendrite allostasis gate` (and `chronos finish`, and `loom-gate complexity`) measure cyclomatic and cognitive complexity for every language with `dendrite-core`, one implementation for the whole ecosystem. Limits are `[complexity] max_cyclomatic` (10) and `max_cognitive` (15) in `.github/dendrite/directives.toml`.
2. **The standard tool of each language**, in `.github/workflows/complexity.yml` of every repository. It reads the same limits from the directives file, so there is one place to change them.

| Language | Tool | What it checks |
|---|---|---|
| Go | `gocyclo`, `gocognit` | cyclomatic and cognitive; `gocyclo` agreed with the removed Go analyzer, and the analyzer agreed with `dendrite-core` on all 2,955 functions of this repository (see `core-parity.md`) |
| Rust | `cargo clippy` with `clippy::cognitive_complexity`, and `lizard` | cognitive (clippy) and cyclomatic (lizard); clippy has no cyclomatic lint |
| Python | `ruff` rule `C901` | McCabe complexity |
| TypeScript, JavaScript | not run | this repository has four Docusaurus site files and the Dendrite check reports nothing for them; add ESLint's `complexity` rule when real application code appears |

## Blocking or report only

A step blocks the pull request where the repository is clean by that tool. Where it is not, the step reports the number of findings as a warning and in the job summary and names the ticket that clears the debt; it turns blocking when that ticket is done. This repository's Go debt is tracked as DEN-816.

The tools can disagree with Dendrite's check in small ways (each counts a few constructs differently). A disagreement is information, not noise: compare the two lists and record the difference on the ticket before changing either side.
