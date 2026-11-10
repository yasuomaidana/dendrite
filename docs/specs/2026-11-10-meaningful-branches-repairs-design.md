# Meaningful branches, repair chains and disposal

## Context

Branches named `agent/den-874` and a shared `develop` say nothing about what they hold. When a dependency is
temporarily pointed at a fork or branch to fix a bug or a security hole, nothing records who depends on which
repair, nothing points the consumers back afterwards, and the leftovers stay on disk and on the remote.

Outcome: every branch and working copy is named after its feature and ticket, features reach `main` by pull
request, Dendrite tracks chains of temporary dependency repairs (ConsumerA → R1 → R2) through to their
restoration, and asks to delete what is no longer needed.

## Refined prompt

> Replace Dendrite's `agent/<ticket>` + `develop` model with feature folders. Each top-level ticket owns a
> trunk `<TICKET>-<slug>/main`; its sub-tickets work on `<ROOT>-<slug>/<TICKET>-<slug>` and merge into the
> trunk; a finished trunk goes to `main` by pull request (agents never push `main`). This is the default in
> every governed repository; `develop` gets one last PR to `main` and is retired.
> Add *repairs*: a recorded, temporary redirection of a dependency (registry package or repository) to a
> meaningfully named branch, in the dependency's own repository or a fork. Repairs chain (R1 may need R2).
> Dendrite shows the chain, opens the PRs bottom-up, and restores each consumer in two stages: on merge to the
> upstream default branch at the merged commit, on release to the released version.
> When a repair or feature is merged and nothing points at it, Dendrite asks whether to delete its local
> working copies and remote branches. It never deletes a fork and never deletes unmerged or unpushed work.

## Use cases

Decided by the project owner: UC1 name shape, UC2 pull request to main, UC5 both stages, UC6 scope (local
copies, remote repair branches, finished feature folders; not forks), rollout (default everywhere, develop
retired). Everything marked *(agent's call)* was chosen by the implementing agent and is open to change.

- **UC1 Lone ticket.** `dendrite work new DEN-890` → branch `DEN-890-fix-login-crash/main`, copy
  `dendrite-work/Dendrite-DEN-890-fix-login-crash`. Slug from the Plane title (existing `featurebranch.Slug`).
  Plane unreachable → `DEN-890/main` *(agent's call)*.
- **UC2 Feature with sub-tickets.** `work new DEN-881` (parent DEN-880) → `DEN-880-branch-naming/DEN-881-slug-names`,
  starting from and merging into `DEN-880-branch-naming/main` (created on first use from `origin/main`). The
  top-most parent names the folder, as today. `chronos finish` on a sub-ticket merges into the trunk; on the
  root ticket it runs the gate, pushes the trunk and opens a PR to `main`; the ticket waits in `to verify`
  until the PR is merged *(agent's call)*.
- **UC3 Retire develop.** `dendrite branch retire develop` opens the final `develop → main` PR. In-flight
  `agent/*` branches get their target recorded as `develop` and finish under the old rules. Once the PR is
  merged and no branch targets `develop`, it is offered for deletion (UC6).
- **UC4 Start a repair.** In ConsumerA on DEN-890: `dendrite repair start embedag`. Dendrite finds the
  dependency's repository, gets a working copy in `dendrite-work/` on a meaningful branch, and points
  ConsumerA's manifest at it with the existing switcher. Repository we can push to (synapse peer) → branch
  there, named after the peer's own ticket filed with `chronos request` (`EMBEDAG-12-fix-x/main`); otherwise a
  fork, branch named after the consumer ticket (`DEN-890-fix-login-crash/main`) *(agent's call)*. Running
  `repair start` inside R1's copy for R2 adds the next link. The original reference of each link is saved.
- **UC5 See and close the chain.** `dendrite repair status` prints the tree (consumer → dependency, branch,
  PR, stage). `dendrite repair pr` opens PRs leaf-first, each body linking the others. `dendrite repair
  advance` (also run by `sync` and `session resume`) moves each link forward:
  `repairing → merged` (PR merged: consumer pinned to the upstream default branch at the merge commit; the
  repair branch is now disposable) `→ released` (a tag containing the merge commit exists and resolves:
  consumer set to that version, or to its original repository reference; link closed). A link advances only
  after the links below it have at least merged.
- **UC6 Disposal.** After `advance`, `session resume` and `checkpoint`, Dendrite lists what is disposable and
  asks once per group: repair working copies, remote repair branches, finished feature folders (trunk merged
  to `main`: local and remote trunk and child branches), a retired `develop`. Dirty, unpushed or unmerged
  items are listed as kept with the reason. Non-interactive runs print the list and `dendrite work dispose
  --yes` instead of asking. Merged, clean worktrees keep being removed automatically as today *(agent's call)*.

Non-goals: deleting forks; merging to `main` by agents; renaming existing branches; registry publishing.

## Approach

Three parts, built in this order as sub-tickets of DEN-879: A naming (DEN-880), integration branch (DEN-881),
finish by pull request and retiring develop (DEN-882); B repairs (DEN-883, DEN-884); C disposal (DEN-885).
Until DEN-881 lands, a trunk merges into the integration branch the project already has.

### A. Naming and no develop

1. **Names** – `pkg/featurebranch/featurebranch.go`: `Plan` gains `Branch` (the work branch) and `Trunk`;
   `Resolve` returns them for every ticket, not only children; `Name` builds `<ID>-<slug>`; `existing()` also
   matches `<ID>-*/main` so a renamed ticket keeps its folder; legacy `feature/<ID>-*` is still found.
   `Ensure` creates the trunk from the integration branch.
2. **One place for the default** – remove the two `"agent/" + ...` literals (`pkg/workcopy/workcopy.go:208`,
   `cmd/dendrite/soma/work_target.go:77`): `runWorkNew` (`cmd/dendrite/soma/work.go`) passes the plan's branch
   and a readable copy name to `Manager.Create`. `--branch`/`--target` still override.
3. **Ticket from branch** – `ticketlock.TicketOfBranch` (`pkg/ticketlock/claim.go:88`) reads the last segment
   (`<TICKET>-slug`), or the folder when the segment is `main`, and still accepts `agent/<ticket>`.
4. **Policy** – `pkg/branchpolicy/branchpolicy.go` `chooseBranch`: member → directive → `origin/HEAD` → main →
   master; `develop` only when named by directive/member or recorded per branch (`TargetKey`). `Protected()`
   keeps main/master/develop; a trunk is protected for every branch except the root ticket's own claim
   (`pkg/vcs/guard.go`, which also hard-codes `develop` at line 113). `[git] feature_prefix` is ignored for
   new branches.
5. **Finish by PR** – new small `pkg/pullrequest` wrapping `gh pr create/view` behind a function variable
   (same seam style as `switcher.goGet`) so tests never reach GitHub. `chronos finish` uses it for root
   tickets; pushing `main`/`master` stays refused.
6. **Retire** – `dendrite branch retire <branch>` in `cmd/dendrite/soma/branch.go`.
7. **Dev channel** – `synapse.ChannelDev.DefaultRef()` (`pkg/synapse/types.go:91`) returns `main`; `dendrite
   update --dev` gains `--branch <trunk>` to install an unmerged feature *(agent's call; the alternative is to
   drop the dev channel)*.
8. **Words** – the AGENTS.md generator text, the `dendrite-workflow` skill, `work new` help,
   `tests/e2e/lifecycle.sh`, `CHANGELOG.md`.

### B. Repairs

New `pkg/repair` (orchestration only):
- `Link{Consumer, Dependency, Ticket, Original, Current switcher.SwitchRequest, Branch, Remote, Fork bool,
  PR, MergeSHA, Version, Stage}` and a ledger at `~/.dendrite/repairs.json` (global because a chain spans
  repositories; never inside a repository).
- Manifest edits only through `switcher.DependencySwitcher.Switch` (`pkg/pool/switcher`, modes
  published/branch/local/fork, already transactional for go.mod, Cargo.toml, package.json, pyproject.toml).
  The switcher gains a read-only `Current(dir, dep)` so the original reference can be saved.
- Peer lookup through `pkg/synapse` members (`Module`, `Remote`, `PathHint`); peer tickets through the
  existing `chronos request` path; Plane links through `chronos link`.
- Release detection: fetch upstream tags, `git tag --contains <MergeSHA>`, then let the ecosystem resolve the
  version; if it does not resolve, the link stays at `merged`.
- `dendrite repair start|status|pr|advance` in `cmd/dendrite/soma/repair.go`, next to `pool.go`, with `--json`;
  `dendrite branch show` prints the links of the current project.

Git cloning and remote-branch deletion are plumbing that belongs to dendrite-git-rs (`TODO(move:...,DGIT-7)`
in `pkg/workcopy`): they are written as thin calls with the same TODO block, and a `chronos request git-rs`
ticket asks for the sidecar methods.

### C. Disposal

- `pkg/workcopy`: `Disposable(ctx)` lists candidates with a kept-reason, reusing `inspect`, `mergedInto` and
  `gcVerdict`; candidates come from merged trunks (PR state via `pkg/pullrequest`) and ledger links at
  `merged`/`released`.
- `dendrite work dispose [--yes] [--dry-run]` in `cmd/dendrite/soma/work.go`; the prompt is called from
  `AutoCleanWorkCopies` (resume, checkpoint) and from `repair advance`. Remote deletion is
  `git push <remote> --delete <branch>`, never on protected branches, never a fork repository.

## Verification

- Unit tests (table style, temp git repositories, isolated `HOME` and `DENDRITE_WORK_DIR`, `gh` and `go get`
  replaced through their seams; nothing touches the real Plane, GitHub or `dendrite-work/`):
  `go test ./pkg/featurebranch ./pkg/branchpolicy ./pkg/ticketlock ./pkg/workcopy ./pkg/repair
  ./pkg/pullrequest ./pkg/pool/switcher ./cmd/dendrite/soma/...`
- `tests/e2e/lifecycle.sh` updated to the new names and run.
- Scratch trial in the session scratchpad: three local repositories A → R1 → R2 with bare remotes; `work new`,
  `repair start` twice, `repair status`, merge R2 then R1 by hand, `repair advance` (check the manifests at
  each stage), tag a release, `advance` again, `work dispose --dry-run` then `--yes`.
- `dendrite allostasis gate --since origin/develop` per sub-ticket; `dendrite chronos finish` on each.
- Last step, in this repository: `dendrite branch retire develop` (opens the pull request; a person merges it).
