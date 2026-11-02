# Changelog

All notable changes to Dendrite are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.1.0] - 2026-11-01

First release from `develop` to `main` since 2026-10-19 (180 commits). Which commands are new is in [`docs/cli/availability.md`](docs/cli/availability.md).

### Added

- **Ticket lifecycle (DEN-375, DEN-377, DEN-379, DEN-393):** a per-worktree claim and a write lock (`dendrite hook pre-edit`), `chronos claim|comment|comments|decision|handoff|plan push|finish`, and the matching MCP tools. `chronos finish` runs the quality gate and checks the diff against each acceptance criterion.
- **Quality directives (DEN-380):** `dendrite directive` reads and changes `.github/dendrite/directives.toml`; `dendrite allostasis gate` judges a change against it.
- **Retrieval (DEN-1044, DEN-1046, DEN-330):** `dendrite rag` served by the `dendrite-rag` sidecar, graph MCP tools (`dendrite_graph_communities`, `_god_nodes`, `_path`, `_explain`), and `--corpus tickets` to search a project's Plane tickets. `dendrite ask` answers through the sidecar's ask method, code by default and `-d` for documentation.
- **Agent guards (DEN-328):** `dendrite hook pre-tool` stops a raw recursive search once and names the Dendrite equivalent.
- **Local inference (DEN-363, DEN-364, DEN-411):** `spike status|hold|models|bench|prune`, a memory-fit guard that picks the smartest model that fits the budget, and OxiLLaMa session leases.
- **Plane (DEN-353, DEN-354, DEN-370, DEN-340):** the `dendrite-plane` sidecar is required (no Go fallback); `chronos create|update`, `chronos project refresh|remove`, Blocked and Cancelled states.
- **Synapses (DEN-373):** `synapse add|delete|remove-member`, per-synapse peer listing, and `chronos request` to file a ticket in a peer's project.
- **Environment (DEN-366):** `soma secrets ensure|status`, `soma stacks migrate`, `soma migrate-state`.
- **Diagnostics (DEN-326):** `dendrite inter probe`, a read-only pass/fail matrix over the MCP tools; `sensory usage`; `bouton jobs show|watch`.

### Changed

- API keys and vendor settings are configured once in `$DENDRITE_HOME/.env`; a project `.env` no longer overrides them (DEN-1057, DEN-343).
- Plane, Jira and Linear clients, topology, the HTTP server and the inference engines share transport and stream helpers instead of repeating blocks (DEN-419, DEN-444).
- Go files over 500 production lines were split by responsibility (DEN-412, DEN-1053).
- Flags and help text changed in `ask`, `cleft`, `prune`, `quantal`, `sync`, `vesicle` and `work`.

### Fixed

- `quantal --files` commits only the listed files (DEN-1036); `quantal --no-ticket-guard` passes the git pre-commit hook (DEN-393).
- `bouton jobs wait|show|list --ticket` find the job in the ticket's project (DEN-1048).
- Ticket resolution is one shared resolver with no substring matches (DEN-324); the finish gate lists its failing lines (DEN-401) and proves each verdict (DEN-406).
- Tests cannot reach the real Docker or write the real stacks directory (DEN-371).
- The Plane HITL sync reads comments from one `gh issue list`, 55 s down to 2.6 s on 128 tickets (DEN-338).

### Security

- The root `.env`, which held live credentials, is untracked and a test fails the build if an environment file is tracked again (DEN-901, DEN-343). The credentials that were committed before this release must be rotated; DEN-442 tracks that the live salt key equals the leaked master key.

### Quality

- Packages touched by DEN-444 are at 100.0% test coverage. The documented exceptions are in [`docs/quality/coverage-exceptions.md`](docs/quality/coverage-exceptions.md).
- New shared helpers: `pkg/testkit`, `pkg/httpscript`, `pkg/textutil`, `pkg/testmain`, `pkg/tracker/shared.go`.
- Older duplicate blocks that this release did not add remain under DEN-414.
