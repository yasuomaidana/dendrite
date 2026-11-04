# dendrite-core parity with the removed Go analyzers

Dendrite used to compute cyclomatic complexity in Go (`go/ast`, `pkg/ast.AnalyzeFuncComplexity`). That code is deleted; `dendrite-core` (dendrite-core-rs, method `ast.complexity`) does the measuring. This page records the one-off comparison that was run before the deletion, and its limits.

## Result

| What | Value |
|---|---|
| Corpus | every non-test, non-testdata Go file of this repository at `origin/develop` before the change: 489 files, 2.9 MB |
| Functions found by the Go analyzer | 2,955 |
| Same function found by `dendrite-core` (matched by declaration line) | 2,955 |
| Cyclomatic complexity equal | 2,955 (100%) |
| Different | 0 |
| Found only by `dendrite-core` | 0 |

The cyclomatic rules are the same: `if`, `for` and `range`, non-default `case` clauses, `select` communication cases, `&&` and `||`.

## Method

The old `complexity.go` was recovered from `origin/develop` and built as a stand-alone program that prints `AnalyzeFileComplexity` for every file. The same files were sent one by one to a release build of `dendrite-core` (JSON-RPC `ast.complexity`, `lang: "go"`). Results were joined on the declaration line and compared. The comparison code was throwaway and is not kept.

## What changed on purpose

- A method's symbol is its identifier (`Start`), where the Go analyzer printed the receiver as well (`(*Service).Start`). Callers that match symbols by name need to expect that.
- `AnalyzeSourceComplexity`'s cognitive number was a line heuristic. `dendrite-core` implements the SonarSource rules (nesting, `else`, runs of boolean operators, labelled jumps, recursion), so cognitive values are different, and meaningful now.
- Other languages (Rust, Python, TypeScript, JavaScript, C, C++) are measured too.

## Speed: the 3x target is still not met

`docs/ARCHITECTURE.md` of dendrite-core-rs sets a target of at least 3 times faster than the in-process Go providers. Measured on the same corpus (503 non-test Go files of this repository, 3.0 MB; 8-core Apple silicon; release builds):

| Run | Wall time |
|---|---|
| Go analyzer, all files, one process, one thread | about 0.10 s |
| `dendrite-core`, one `ast.complexity` request per file (first measure, DCORE-13) | about 0.40 s |
| same, after DCORE-13: nodes classified by kind id, one cursor per tree | about 0.31 s |
| `dendrite-core`, one `ast.complexity_batch` request for all files | about 0.07 s |

What the profile of the 0.40 s run showed: the time was in walking the tree, not in parsing. Every node asked tree-sitter for its kind name (a C call, `strlen` and a UTF-8 check) several times, then compared it with string lists; and every node allocated its own cursor. `dendrite-core` now resolves the kind names once per language into flags indexed by kind id and walks each tree with one cursor. The output is byte-identical on all 503 files. What is left is mostly tree-sitter parsing (about two thirds of the time), which one thread cannot shorten.

Concurrency does: files are independent, so the sidecar measures a batch on up to one thread per core. With four workers the corpus takes 0.09 s, with all cores 0.07 s, which is **1.4 times faster than the single-threaded Go analyzer**, not 3 times. Reaching 3 times would need 0.033 s, less than parsing 3 MB costs on this machine. A caller that measures one file per request has nothing to run in parallel; `assert.CheckPackageComplexity` therefore sends a package's files as one batch (`coreclient.ComplexityBatch`). An older `dendrite-core` without the method makes the call fail with `coreclient.ErrBatchUnsupported`, which says to run `dendrite update --dev`.

For gate-sized work (a changed file, a few functions) the difference is milliseconds, and the process is started once per session by `pkg/sidecar`.

## Consequences

- `dendrite-core` is required. `dendrite install --dev` and `dendrite update --dev` build it with the other members, and `dendrite doctor` reports whether it was found. Without it, callers get `coreclient.ErrBinaryMissing`, which says how to install it. There is no Go fallback.
- Folding (`ast.fold_signatures`) was not compared against the old Go folding output; the Rust side has its own tests (the output must still parse, bodies become a placeholder, kept symbols stay).
