# Changelog

All notable changes to Dendrite are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- **Branches are named after their feature and ticket (DEN-879, DEN-880):** `dendrite work new` no longer creates `agent/<ticket>`. A ticket works on its own trunk `<TICKET>-<slug>/main` in the copy `dendrite-work/<repo>-<TICKET>-<slug>`; a sub-ticket works on `<ROOT>-<slug>/<TICKET>-<slug>` and merges into the trunk `<ROOT>-<slug>/main` of its top-most parent. The slug is the Plane title; without Plane the branch is `<name>/main`. Existing `agent/<ticket>` branches and `feature/<ROOT>-<slug>` trunks keep working, `dendrite work done DEN-123` finds the copy by its ticket, and `dendrite branch show --ticket` prints the work branch. The design is in [`docs/specs/2026-11-10-meaningful-branches-repairs-design.md`](docs/specs/2026-11-10-meaningful-branches-repairs-design.md).
- **The integration branch defaults to `main` (DEN-879, DEN-881):** with no directive and no synapse member branch, work integrates on the remote's default branch, then `main`, then `master`; `develop` is chosen only when it is the one of them the repository has, or when `[git] integration_branch` or the member names it. In a repository that still has `develop`, branches made before feature folders (`agent/*`, `feature/*`) keep `develop` as their target (`dendrite branch show` says "branch made before feature folders"). The dev channel still tracks `develop`; the `dendrite vcs` guard is unchanged (DEN-886).
- **A branch that merges into `main` is finished by pull request (DEN-879, DEN-882):** `dendrite chronos finish` pushes such a branch and opens its pull request with `gh` (`pkg/pullrequest`); the ticket waits in To Verify for a person to merge it, and `main`/`master` are still never pushed. When `gh` cannot open it, the finish note gives the command. The generated rules and the `dendrite-workflow` skill describe this for projects that integrate on `main`, and the direct merge for the others.

### Added

- **`dendrite branch retire <branch>` (DEN-882):** opens the last pull request of a branch such as `develop` into the integration branch, lists the local branches that still merge into it, and says when nothing uses it any more.
- **`dendrite repair start <dependency>` (DEN-879, DEN-883):** starts a temporary repair of a dependency. It gets a working copy of the dependency in `dendrite-work/` on a branch named after the ticket (`<TICKET>-<slug>/main`; a synapse peer is repaired in its own repository under `--dep-ticket`, anything else from `--repo`, pushed to `--fork` when given), pushes the branch, points this project's manifests at it and records the link, with how the dependency was named before, in `~/.dendrite/repairs.json` (`pkg/repair`). Run inside a repair's working copy it adds the next link of the chain. `go.mod` is pointed at the working copy, because a Go module cannot name a branch of another repository without a resolved version. `switcher.Current` and `switcher.Restore` read and put back a manifest's entry.
- **`dendrite repair status|pr|advance|drop` (DEN-879, DEN-884):** `status` prints every repair chain as a tree (consumer, dependency, branch, pull request, stage) and `dendrite branch show` lists the repairs of the current project. `pr` opens the pull requests, the bottom of each chain first, each naming the ones it waits for. `advance` moves a repair in two stages: once its pull request is merged the consumer is pinned to the dependency's repository at the merge commit (`rev` in Cargo.toml and pyproject.toml), and once a tag holds that commit a consumer that used a published version gets the release while any other gets back exactly what it had; a repair waits for the repairs below it to be merged. `dendrite session resume`, `session checkpoint` and `dendrite sync` advance repairs and print the ones that moved. `drop` forgets a repair. A release is detected from the repository's tags; whether the registry already serves it is not checked.
- **`dendrite work dispose` (DEN-879, DEN-885):** lists what merged work left behind and asks once per group before deleting it: the working copies and pushed branches of repairs whose fix is merged, the branches (local and on the push remote) of features whose trunk is merged into the integration branch (by a merge commit, or by a pull request GitHub reports as merged; a trunk nothing was committed on yet is not one), and a retired `develop` (`pkg/dispose`). Anything with uncommitted, unpushed or unmerged work, a branch that is checked out, and a `develop` that a branch or a synapse still uses are listed as kept, with the reason; a fork repository is never deleted. `--dry-run` only lists, `--yes` deletes without asking, and without a terminal nothing is deleted. `dendrite session resume`, `session checkpoint` and `dendrite repair advance` make the same offer when something can be deleted, without asking the network.

## [0.1.0] - 2026-11-01

First release from `develop` to `main` since 2026-10-19 (180 commits). Which commands are new is in [`docs/cli/availability.md`](docs/cli/availability.md).

### Added

- **Every pull request runs CI, and Rust projects are held to Diesel (DEN-874, DEN-875):** the generated workflow triggers on every `pull_request` without a branch or path filter (push still runs for the integration branch, `main` and feature branches). A project with a Rust component also gets the job `db-access (diesel only)`, which runs `scripts/check-db-access.sh` (written by `soma prepare`): it rejects sqlx, rusqlite, sea-orm, mysql and mysql_async, a bare postgres, tokio-postgres or libsql driver without a diesel crate, and `sql_query` outside a `raw` module; exceptions go in `.github/dendrite/db-access.allow`. Dendrite's own CI checks the plane sidecar out from `main`.
- **Ticket lifecycle (DEN-375, DEN-377, DEN-379, DEN-393):** a per-worktree claim and a write lock (`dendrite hook pre-edit`), `chronos claim|comment|comments|decision|handoff|plan push|finish`, and the matching MCP tools. `chronos finish` runs the quality gate and checks the diff against each acceptance criterion.
- **Quality directives (DEN-380):** `dendrite directive` reads and changes `.github/dendrite/directives.toml`; `dendrite allostasis gate` judges a change against it.
- **Retrieval (DEN-1044, DEN-1046, DEN-330):** `dendrite rag` served by the `dendrite-rag` sidecar, graph MCP tools (`dendrite_graph_communities`, `_god_nodes`, `_path`, `_explain`), and `--corpus tickets` to search a project's Plane tickets. `dendrite ask` answers through the sidecar's ask method, code by default and `-d` for documentation.
- **Agent guards (DEN-328):** `dendrite hook pre-tool` stops a raw recursive search once and names the Dendrite equivalent.
- **Local inference (DEN-363, DEN-364, DEN-411):** `spike status|hold|models|bench|prune`, a memory-fit guard that picks the smartest model that fits the budget, and OxiLLaMa session leases.
- **Plane (DEN-353, DEN-354, DEN-370, DEN-340):** the `dendrite-plane` sidecar is required (no Go fallback); `chronos create|update`, `chronos project refresh|remove`, Blocked and Cancelled states.
- **Synapses (DEN-373):** `synapse add|delete|remove-member`, per-synapse peer listing, and `chronos request` to file a ticket in a peer's project.
- **Environment (DEN-366):** `soma secrets ensure|status`, `soma stacks migrate`, `soma migrate-state`.
- **Diagnostics (DEN-326):** `dendrite inter probe`, a read-only pass/fail matrix over the MCP tools; `sensory usage`; `bouton jobs show|watch`.
- **Syntax analysis in Rust (DEN-813):** `pkg/coreclient` talks to the `dendrite-core` sidecar (dendrite-core-rs) for complexity, signature folding, last complete node, changed symbols, token estimates and chunk identity. `pkg/assert`, `pkg/ai/importer` and `pkg/ai/tokenopt` use it. `dendrite-core` is required, with no Go fallback; `dendrite soma doctor` reports it.
- **Complexity and structure gates (DEN-812):** directives `[complexity]` (`max_cyclomatic` 10, `max_cognitive` 15) and `[structure]` (`max_files_per_dir` 12, `max_dir_production_lines` 3000, `max_prefix_group` 4, `require_docs`, `docs_files`) and the `complexity` and `structure` checks of `dendrite allostasis gate` and `chronos finish`, run on the `dendrite-rag` sidecar (protocol 1.12). The structure check also requires a README and a CHANGELOG for every crate, module and repository root.
- **Second opinion in CI (DEN-815):** `.github/workflows/complexity.yml` cross-checks complexity with `gocyclo` and `gocognit` (report only while DEN-816 is open) and `ruff` C901, reading the limits from the directives; see `docs/quality/complexity-second-opinion.md`.
- **Pending moves (DEN-814, DEN-818):** Go files that will move to a Rust repository or to dendrite-mcp-hub carry a `TODO(move:<repo>,<TICKET>)` block (what, when, what stays), listed in `docs/architecture/subprojects.md`, which also states what each sub-project implements today.
- **Skill scan gate (DEN-425):** `dendrite immune scan <path>` runs SkillSpector on a skill and prints approve, hold or quarantine from its risk score; the bands are the `[immune]` directives. `immune update` and `immune enforce` are not part of this release.

### Changed

- **The Go complexity, folding and token analyzers in `pkg/ast` and `pkg/ai/tokenopt` are deleted (DEN-813).** A one-off comparison on 489 files and 2,955 functions found identical cyclomatic values (`docs/quality/core-parity.md`). Re-measured after DCORE-13 (which replaced tree-sitter for Go with a lexer scan), the sidecar takes about 19 ms against about 99 ms for the removed analyzer on that corpus, about 5 times faster, so the 3x target is met; the first measurement, 0.40 s against 0.10 s, is kept in the page. `pkg/ast` keeps the symbol graph until it moves (DRAG-79).
- API keys and vendor settings are configured once in `$DENDRITE_HOME/.env`; a project `.env` no longer overrides them (DEN-1057, DEN-343).
- Plane, Jira and Linear clients, topology, the HTTP server and the inference engines share transport and stream helpers instead of repeating blocks (DEN-419, DEN-444).
- **`cmd/dendrite` is one package per command group (DEN-839),** with shared code in `internal/cli`, `internal/clitest` and `internal/planecli`; the help output of all 274 commands is unchanged. `dendrite-core-rs` also now covers Java, Kotlin and Svelte and caches batch results (DCORE-20, DCORE-13), and the sidecar speed table in `docs/quality/core-parity.md` uses like-for-like numbers (DEN-838).
- Go files over 500 production lines were split by responsibility (DEN-412, DEN-1053).
- Flags and help text changed in `ask`, `cleft`, `prune`, `quantal`, `sync`, `vesicle` and `work`.

### Fixed

- `quantal --files` commits only the listed files (DEN-1036); `quantal --no-ticket-guard` passes the git pre-commit hook (DEN-393).
- `bouton jobs wait|show|list --ticket` find the job in the ticket's project (DEN-1048).
- Ticket resolution is one shared resolver with no substring matches (DEN-324); the finish gate lists its failing lines (DEN-401) and proves each verdict (DEN-406).
- Tests cannot reach the real Docker or write the real stacks directory (DEN-371).
- The Plane HITL sync reads comments from one `gh issue list`, 55 s down to 2.6 s on 128 tickets (DEN-338).
- `DEV_STATUS.md` lists only the tickets of its own Plane project. Since DEN-984 it also pulled in every other project of the workspace, and copies of other projects' tickets that sat on the Dendrite board are no longer listed (DEN-828). A peer's file gets its own project name.
- `soma prepare` registers the Dendrite MCP servers with Antigravity (`agy mcp add`), which ignores `.agents/mcp.json`, and `soma doctor` reports whether they are registered (DEN-830).

### Known limits

- The new `complexity` and `structure` checks fail on this repository when run over everything: 829 functions are above cyclomatic 10 or cognitive 15 (DEN-824) and `cmd/dendrite`, `pkg/mcpserver`, `pkg/tracker/plane`, `pkg/ticketlock`, `pkg/env/bootstrap` and `pkg/devtool` hold more modules than the structure check allows. The complexity check judges only what a change touches. The structure check does not, so `chronos finish` keeps reporting its findings (and stops short of Done) until DEN-822, DEN-823 and DEN-829 split those packages.
- Dendrite itself is not on the generated CI workflow yet (DEN-824), and the peers (mcp-hub, rag-rs, plane-rs, embedag) have not adopted it either. These and the move-notice tooling (DEN-827) are the 0.2.0 scope.
- `dendrite-git` is built and installed with the other sidecars, but no Go code calls it yet (DGIT-1, DGIT-4, DGIT-7); `soma doctor` says so.
- Go code that duplicates a Rust sub-project is marked `TODO(move:...)` and listed in `docs/architecture/subprojects.md`; it is not removed yet.

### Security

- The root `.env`, which held live credentials, is untracked and a test fails the build if an environment file is tracked again (DEN-901, DEN-343). The credentials that were committed before this release must be rotated; The keys were rotated and DEN-442 is closed.
- `dendrite cortex serve` requires a generated access token (printed in the URL it opens; `--no-auth` turns it off), refuses foreign `Host` and `Origin` headers on HTTP, WebSocket and SSE, sends no CORS headers and accepts only `application/json` bodies (DEN-862).
- The compose stacks publish every port on `127.0.0.1` (`DENDRITE_BIND_HOST` changes it), Redis requires a generated `REDIS_PASSWORD`, and the LiteLLM image is pinned by digest (DEN-863).
- `dendrite quantal` pushes `HEAD:refs/heads/<branch>` to an explicit destination, so a branch that tracks `origin/main` cannot reach `main` (DEN-864).
- `/api/v1/telemetry/snapshot` and the dashboard report measured values, or list a field as `unavailable`, instead of fixed numbers; `/api/v1/health` reports version `0.1.0` (DEN-865).
- `dendrite ask --server` sends the central server's access token as a Bearer header (`--server-token`, else `DENDRITE_RAG_TOKEN`) and explains a 401 instead of failing opaquely (DEN-868).
- The dashboard session cookie holds a random server-side session id (HttpOnly, SameSite=Strict, 12 h, one name per port) and never the access token, so another service on localhost learns nothing reusable (DEN-869).
- The telemetry snapshot keeps each frame's own timestamp and flags frames older than 30 s as `stale`; the on-demand temperature is read at most every 30 s with a 3 s timeout, and a zero or failed reading is listed as `unavailable` (DEN-870).
- `dendrite quantal` picks the remote in this order: a synapse member's `push_remote`, `branch.<name>.pushRemote`, `remote.pushDefault`, the tracked remote, `origin`; the destination is still the branch's own name (DEN-871).

### Quality

- Packages touched by DEN-444 are at 100.0% test coverage. The documented exceptions are in [`docs/quality/coverage-exceptions.md`](docs/quality/coverage-exceptions.md).
- New shared helpers: `pkg/testkit`, `pkg/httpscript`, `pkg/textutil`, `pkg/testmain`, `pkg/tracker/shared.go`.
- Older duplicate blocks that this release did not add remain under DEN-414.
