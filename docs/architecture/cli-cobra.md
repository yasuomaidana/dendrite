---
id: cli-cobra
title: "CLI command tree (cobra)"
sidebar_label: "CLI command tree"
---

# CLI command tree (cobra)

`cmd/dendrite` is a [cobra](https://github.com/spf13/cobra) command tree built by `newRootCmd()` (`cmd/dendrite/root.go`). Tracking ticket: DEN-950.

## Naming

- The **Neuro-Lore name is the primary name** (`Use`). The plain name and the other lore names are `Aliases`.
  - For example, `chronos` has the aliases `plane`, `chronos-tracker` and `pln`.
  - Lookup is case-insensitive.
- Each top-level command belongs to one help group (`GroupID`). The groups are the neuro categories listed in `commandGroups` in `root.go`.
- Each feature lives at **one** path. Duplicate or legacy paths are deleted, not kept as hidden aliases.

## Global flags

These are persistent flags on the root command, read through the `globals` variable (`cmd/dendrite/flags.go`):

| Flag | Meaning |
| :--- | :--- |
| `--json` | Machine-readable output. Read `globals.JSON` and do not declare a local `--json`. |
| `-y, --yes` | Non-interactive. Read `globals.Yes` wherever the command would prompt. |
| `--state-dir` | Exported as `DENDRITE_STATE_DIR` before `RunE` runs. `statedir.Resolve` picks it up. |
| `--home` | Exported as `DENDRITE_HOME`. |
| `--gateway-url` | Exported as `LITELLM_PROXY_URL`. Use `gatewayV1URL()` / `gatewayBaseURL()` and never hardcode `localhost:4000`. |
| `--plane-url` | Exported as `PLANE_BASE_URL`. `plane.NewClientFromEnv` picks it up. |

Secrets such as `PLANE_API_KEY` and `LITELLM_MASTER_KEY` are never flags. They are configured once in `$DENDRITE_HOME/.env`; before any command runs, `applyGlobals` calls `dotenv.Load` (`pkg/dotenv`), which sets every variable the environment does not export, from the project's own `.env` (nearest git root) first, then from the global file. Code that resolves a key for another project (`dendrite ask --project`, agent jobs in a worktree) uses `dotenv.Lookup(dir, key)` and never reads `.env` files itself. Tests of `cmd/dendrite` set `DENDRITE_DOTENV=off`.

## Writing a command

One file per lore command: `cmd/dendrite/<plain>.go` exports `new<Plain>Cmd() *cobra.Command`, and `newRootCmd()` adds it.

- **Parents** come from `newGroupCmd(use, short, groupID, aliases...)`. Called with no args, a parent prints help (exit 0). Called with an unknown subcommand, it returns a usage error with suggestions (exit 2). Subcommands have an empty group ID.
- **Leaves** follow these rules:
  - Use `RunE`, never `Run`.
  - Validate positionals with `noArgs`, `exactArgs(n)`, `minArgs(n)`, `maxArgs(n)` or `rangeArgs(lo, hi)`. These are usage-aware wrappers around cobra's validators, so violations exit 2.
  - Bind flags to an options struct with pflag (`cmd.Flags().StringVarP(...)`).
  - Mark required flags with `requireFlags(cmd, "name")`.
  - Repeatable string flags use `StringArrayVar`. Comma lists use `StringSliceVar`.
  - Reuse the shared helpers: `addDryRunFlag`, `addTicketFlag` (`--ticket/-t`), `addModelFlag`, `addProjectFlag`, `addFileFlag` (`--file/-f`).
  - Never reuse a shorthand for a different meaning inside one subtree, and never take `-y`, `-v` or `-h`.
- **Output** goes to `cmd.OutOrStdout()` / `cmd.ErrOrStderr()` and is passed down as an `io.Writer`, so tests can capture it.
- **Errors:** return them, never call `os.Exit`, and never print-and-return-nil.
  - Wrap invocation mistakes found after parsing with `usageErrorf(...)`.
  - Every other error exits 1, and `main.go` prints it once as `[-] <err>`.
- **Help text:** every command sets `Short`. Parents and non-obvious leaves set `Example`, with one invocation per line using the lore name. `dendrite codex generate --reference` renders these into `docs/cli/<name>.md`, and `codex audit --strict` fails CI when the docs are stale.
- **Tests** run the real tree through `runCLI(ctx, &buf, args...)` (`cmd/dendrite/root_test.go`). They check output and `exitCodeFor(err)`. A command that is not wired into `newRootCmd()` yet is tested with `runWith(ctx, &buf, newXCmd(), "<name>", args...)`. This mounts the command on a bare root that has the global flags.

## Example: before and after

Before (hand-parsed, exit code always 0):

```go
func handleRefCommand(ctx context.Context, action string, args []string) {
	switch action {
	case "remove", "rm":
		if len(args) < 1 {
			printRefUsage()
			return
		}
		if err := defaultRefMgr.Remove(args[0]); err != nil {
			fmt.Printf("[-] %v\n", err)
			os.Exit(1)
		}
	}
}
```

After:

```go
func newRefCmd() *cobra.Command {
	cmd := newGroupCmd("afferent", "Manage Tier 1 source references", "arbor", "ref", "dendritic-afferent", "aff")
	cmd.AddCommand(&cobra.Command{
		Use:     "remove <id>",
		Aliases: []string{"rm"},
		Short:   "Remove a source reference",
		Args:    exactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := defaultRefMgr.Remove(args[0]); err != nil {
				return fmt.Errorf("remove reference %s: %w", args[0], err)
			}
			fmt.Fprintf(c.OutOrStdout(), "[✓] Removed %s\n", args[0])
			return nil
		},
	})
	return cmd
}
```

Reference implementations: `session.go` and `synapse.go`.

## State and seams

- **No package-level state:** commands build their managers, stores and clients inside `RunE` (`newRefManager()`, `newPoolManager()`, `openSegmentedStore()`, `newForkController()`, ...). A value resolved at package init would miss `--state-dir`, which is only exported in `PersistentPreRunE`.
- **Test seams** are package variables holding a constructor: `newPlaneClient`, `newProjectRegistry`, `openSegmentedStore`, `newWorkManager`, `runCoverage`, `runCompose`, `jetbrainsMCPURL`. Tests swap them with `t.Cleanup` to restore. Never name test types or helpers "mock", "fake" or "stub": the commit gate rejects them. Prefer real `httptest` servers and temp dirs.

## Paths other code depends on

- `pkg/agentjobs` re-executes `<binary> bouton jobs run <id>` (hidden leaf), and the LaunchAgent runs `bouton jobs daemon`.
- The IDE run configurations and external tools that `bridge setup` writes (`pkg/jetbrains/setup.go`) and `.ai/mcp/mcp.json` call `inter serve`.
- CI and the pre-commit hook run `allostasis coverage --ratchet` and `codex audit --strict`.

Renaming any of these needs the caller changed in the same commit. The plain-name aliases keep older installs working.
