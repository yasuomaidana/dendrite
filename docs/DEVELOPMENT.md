# Developing Dendrite Across Its Repositories

Dendrite (this repository, `dendrite-control`) is split across sibling repositories. [architecture/subprojects.md](architecture/subprojects.md) lists who owns what. On a development machine they sit next to each other:

```text
~/GoLandProjects/
  Dendrite/              # dendrite-control (Go, module "Dendrite")
  dendrite-mcp-hub/      # MCP server core (Go module)
  dendrite-core-rs/      # tree-sitter/AST sidecar (Rust)
  dendrite-git-rs/       # git analysis sidecar (Rust)
  dendrite-rag-rs/       # retrieval sidecar (Rust)
~/RustRoverProjects/
  embedag/               # graph/DAG library used by the Rust crates
```

A **synapse** ties them together. It is a manifest in `~/.dendrite/synapses/<id>.toml` that records each repository's identity, role, tracked branch, and whether it is consumed from your **local** checkout or from its **remote**.

## 1. Link the repositories once

```bash
cd ~/GoLandProjects/Dendrite
dendrite synapse create dendrite-stack --sync go.work \
  --members .=control:consumer,../dendrite-mcp-hub=mcphub:producer
dendrite synapse create dendrite-rust --sync cargo-patch \
  --members ../dendrite-core-rs=core:consumer,../dendrite-rag-rs=rag:consumer,$HOME/RustRoverProjects/embedag=embedag:producer
dendrite synapse set-source dendrite-rust embedag local --module embedag \
  --remote https://github.com/yasuomaidana/embedag
```

Repositories outside `~/GoLandProjects`, `~/RustRoverProjects` and the other IDE folders can still be found after a move. Set `DENDRITE_SEARCH_ROOTS=~/work:~/src` so that `dendrite synapse heal` searches there.

## 2. Local vs. remote sources (develop mode)

Each member is either `local` (the default) or `remote`:

| Source | Go consumers (`go.work`) | Rust consumers (`cargo-patch`) | Sidecar binaries |
|---|---|---|---|
| `local` | `use ../member` in the consumer's generated `go.work` | `[patch."<remote>"] crate = { path = ... }` block in `.cargo/config.toml` | `member/target/release/<bin>` is found first |
| `remote` | dropped from `go.work`; `go.mod`'s `require` decides | patch block removed; `Cargo.toml`'s git dependency decides | `$DENDRITE_HOME/bin`, `PATH`, `~/.cargo/bin` |

Switch a single module without touching the others:

```bash
dendrite synapse set-source dendrite-stack mcphub remote --ref develop   # use the published branch
dendrite synapse set-source dendrite-stack mcphub local                  # back to ../dendrite-mcp-hub
dendrite synapse sync dendrite-stack                                     # regenerate go.work / patches
```

A mixed setup works the same way. For example, `mcphub` can be local while `embedag` comes from `remote --ref v0.4.0`.

`go.work` is per-developer state and is gitignored. A `go.work` that you wrote by hand is never overwritten.

## 3. Pull other people's changes

```bash
dendrite synapse status dendrite-stack     # fetch only; shows ahead/behind, dirty files, drift
dendrite synapse sync dendrite-stack       # fast-forward, lock commits, relink
```

- **`sync` fast-forwards only.** A member is updated only when its checkout is on the branch it tracks and has no uncommitted tracked changes. The tracked branch is the member's `ref`, else its synapse channel's branch (see section 4), else the checkout's upstream. Use `--autostash` to stash around the update and `--rebase` to replay local commits. Plain `sync` never switches branches: a member on a different branch is reported as drift.
- **`sync --enforce` switches.** A clean checkout on another branch is moved onto its tracked branch (created from `origin/<branch>` if needed) and fast-forwarded. A checkout with uncommitted changes is refused, never stashed across a switch. Working copies under `dendrite-work/` stay on their agent branches. `dendrite install` and `dendrite update --dev` on a synapse with a channel always enforce.
- **Other branches.** `status` lists remote branches whose commits are in neither your checkout nor the tracked branch, for example a teammate's `feat/x` on `dendrite-mcp-hub`, and prints the command to take them: `dendrite synapse sync dendrite-stack --merge mcphub=feat/x`. A conflicting merge is aborted and leaves the tree clean.
- **Remote members** are resolved with `git ls-remote`, without a checkout.
- **Locks.** Every member's resulting commit is written to the manifest (`locked_sha`), so `inspect` shows exactly which combination you are running. The synapse becomes `drifted` while anything is behind or blocked, `broken` when a checkout is missing (run `dendrite synapse heal`), and `active` again once everything is in sync.
- **Pushing.** `dendrite sync` (feat/synapse-sync) is the outbound counterpart. It pushes unpushed commits and refreshes Plane for every peer.

## 4. Install: git repositories and branches

The ecosystem is distributed as git repositories, not as release binaries. Its definition (members, remotes, roles) is committed in `pkg/ecosystem/dendrite-ecosystem.toml` and embedded in the binary. A channel decides which branch every member tracks:

| Channel | Branch | Install with |
|---|---|---|
| `stable` (default) | `main` | `scripts/install.sh` or `dendrite install` |
| `dev` | `develop` | `scripts/install.sh --dev` or `dendrite install --dev` |

```bash
# fresh machine: clones Dendrite, then every sub-project next to it under ~/dendrite
curl -sSL https://raw.githubusercontent.com/yasuomaidana/dendrite/main/scripts/install.sh | bash
curl -sSL https://raw.githubusercontent.com/yasuomaidana/dendrite/develop/scripts/install.sh | bash -s -- --dev

# from an existing checkout: members are cloned next to it (or found where the synapse says)
dendrite install --dev
dendrite install --dev --ref rag-rs=feat/ann   # one sub-project on another branch
dendrite install --dev --ref rag-rs=           # back to the channel's branch
dendrite synapse channel dendrite-ecosystem    # show the channel and every override
```

`install` clones missing members at their tracked branch and keeps existing locations (for example a member under `~/RustRoverProjects`). It records the channel, overrides and paths in `~/.dendrite/synapses/dendrite-ecosystem.toml`, then enforces the branches as `synapse sync --enforce` does. Finally it builds the members into `$DENDRITE_HOME/bin` and rebuilds `dendrite`. A dirty member stops the install, and nothing is stashed. Re-running `install` without `--dev`/`--stable` keeps the stored channel. Per-member overrides can also be set with `dendrite synapse set-source dendrite-ecosystem <alias> local --ref <branch>`.

## 5. Update the toolchain

```bash
dendrite update --dev            # from inside the Dendrite checkout
```

1. Runs `synapse sync` (pull and link) for every synapse the checkout belongs to. A synapse with a channel is synced with `--enforce`.
2. Builds every local member that ships a binary:
   - Go `main` packages go to `$DENDRITE_HOME/bin`.
   - Rust crates are built with `cargo build --release` and stay in `target/release`, where the sidecar locator looks.
   - Library-only repositories are skipped.
3. Builds `./cmd/dendrite` next to the installed binary, then swaps it in. The old binary is kept as `dendrite.bak`, and the new one must pass `dendrite version` or the swap is rolled back.

Flags:

| Flag | Effect |
|---|---|
| `--synapse <id>` | Limit the update to the named synapse(s). |
| `--no-sync` | Skip fetching and fast-forwarding. |
| `--no-members` | Skip building sibling binaries. |
| `--target <path>` | Choose the binary to replace. The default is the running `dendrite`, or `$GOBIN`/`$GOPATH/bin` when run through `go run`. |
| `--source-dir` / `DENDRITE_SRC` | Build a checkout other than the current one. |

`dendrite update` without `--dev` downloads the latest GitHub release asset `dendrite_<os>_<arch>` and verifies it against `dendrite_<os>_<arch>.sha256` before the same swap. No release workflow publishes these yet. Until one does, it falls back to `dendrite install` on the stored channel.

`dendrite env update` and `dendrite project update` are aliases of `prepare`. They re-scaffold the project and do not update the binary.

## 6. Sidecars

The Rust repositories ship `dendrite-<x>` binaries that speak the [sidecar protocol](architecture/sidecar-protocol.md). Dendrite always has a Go fallback, so a missing or incompatible sidecar only means the slower path. To point at a specific build, set `DENDRITE_DENDRITE_CORE_BIN=/path/to/dendrite-core`. The same pattern works for any sidecar, including `DENDRITE_OXILLAMA_BIN`.

## 7. Tests touching git

Tests must never run git commands against the developer's checkout. Create a scratch repository, together with a bare origin when needed, and `t.Chdir` into it. `pkg/mcpserver`'s `chdirToScratchRepo` is an example. A test that runs the branch guard in `.` once switched contributors' branches to `develop`.
