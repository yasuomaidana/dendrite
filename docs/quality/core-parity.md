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

## Speed: the target is met

`docs/ARCHITECTURE.md` of dendrite-core-rs sets a target of at least 3 times faster than the in-process Go providers. The first measurement, taken when the Go analyzer was deleted, missed it: the same corpus took about 0.10 s in Go and about 0.40 s through the sidecar, which was then about 4 times **slower**, because every Go file went through tree-sitter. DCORE-13 closed the gap: `ast.complexity` for Go no longer builds a syntax tree (a lexer and a structural scan give the same metrics, and fall back to the parser for what a token scan cannot decide).

Measured again at dendrite-core-rs `f315402`, on the same 489 files (2.9 MB, recovered from `origin/develop` before DEN-813), release build, Apple silicon, 20 runs after one warm-up:

| Run | Wall time (best / median) |
|---|---|
| Go analyzer (`AnalyzeFileComplexity`, recovered from the commit before DEN-813), all 489 files, one process including its start | 87 ms / 99 ms |
| `dendrite-core`, the same files as 489 JSON-RPC `ast.complexity` requests on stdin, one process including its start | 18 ms / 19 ms |

The sidecar finds the same 2,955 functions, with no errors, and is about 5 times **faster**: the 3x target is met. The two runs are not identical work, in both directions: the sidecar also computes cognitive complexity, parses 3.2 MB of JSON requests and writes JSON responses, while the Go program only walked the AST but read the files from disk. The first run after a fresh build took 0.36 s; every later run was within 18 to 20 ms. A batch request (`ast.complexity_batch`, protocol 1.1.0) and the in-memory figures are in `docs/ARCHITECTURE.md` of dendrite-core-rs.

For gate-sized work (a changed file, a few functions) the difference is milliseconds, and the process is started once per session by `pkg/sidecar`. Other languages still go through tree-sitter and were not part of this comparison.

## Consequences

- `dendrite-core` is required. `dendrite install --dev` and `dendrite update --dev` build it with the other members, and `dendrite doctor` reports whether it was found. Without it, callers get `coreclient.ErrBinaryMissing`, which says how to install it. There is no Go fallback.
- Folding (`ast.fold_signatures`) was not compared against the old Go folding output; the Rust side has its own tests (the output must still parse, bodies become a placeholder, kept symbols stay).
