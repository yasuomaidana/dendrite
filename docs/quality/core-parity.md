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

## Speed: the 3x target is met for Go complexity

`docs/ARCHITECTURE.md` of dendrite-core-rs sets a target of at least 3 times faster than the in-process Go providers. The first measurement said the opposite (the sidecar was about 4 times slower), and an intermediate version of this page claimed "1.4 times faster" against a stale Go figure. The like-for-like numbers, from DCORE-13 (499 non-test Go files of this repository, 3.0 MB; 8-core Apple silicon; release builds; sources already in memory; best of several runs):

| Run | Time |
|---|---|
| Go analyzer (`go/parser` and an AST walk), 1 thread | 61 ms |
| same, 8 goroutines | 22 ms |
| same, reading the files from disk as well, 1 thread / 8 goroutines | 88 ms / 30 ms |
| `dendrite-core` through tree-sitter, 1 thread (the first sidecar) | 302 ms |
| `dendrite-core`, Go fast scan, 1 thread | 8.8 ms |
| same, `ast.complexity_batch` on 8 threads | 2.0 ms |

The sidecar's analysis is 7 times faster than the Go analyzer on one thread and 11 times on all cores. Through the sidecar process (JSON in and out, a cold start of about 3 ms) 503 single requests take about 19 ms and one batch request about 13 ms; the sidecar process is started once per session by `pkg/sidecar`, and what remains is the transport of megabytes of JSON, not the analysis.

**Why it was slow, and what changed.** Half of the first 302 ms was walking the tree (kind names compared as strings per node; fixed with numeric kind ids). The other half was the parse: tree-sitter is a general parser that builds a full syntax tree for any grammar, and `go/parser` is a hand-written parser for one language, so the gap is the algorithm and not Rust against Go. `dendrite-core` now analyses Go without a syntax tree (a lexer and a scan of the keywords, braces and operators the metrics read) and falls back to tree-sitter for what it cannot decide from tokens, which is under 0.2% of files. Its answers are identical to the parser's on the Go standard library (7,342 files) and on this repository, and identical on a 34,632-file module cache except for two files where tree-sitter-go misparses valid Go (`if !a.b { ... }` followed by a bare block), where the scan follows Go's own grammar. Other languages still use tree-sitter.

`assert.CheckPackageComplexity` sends a package's files as one `ast.complexity_batch` request (`coreclient.ComplexityBatch`). An older `dendrite-core` without the method makes the call fail with `coreclient.ErrBatchUnsupported`, which says to run `dendrite update --dev`.

## Consequences

- `dendrite-core` is required. `dendrite install --dev` and `dendrite update --dev` build it with the other members, and `dendrite doctor` reports whether it was found. Without it, callers get `coreclient.ErrBinaryMissing`, which says how to install it. There is no Go fallback.
- Folding (`ast.fold_signatures`) was not compared against the old Go folding output; the Rust side has its own tests (the output must still parse, bodies become a placeholder, kept symbols stay).
