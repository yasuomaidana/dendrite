package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"Dendrite/pkg/dotenv"
	"Dendrite/pkg/stacks"

	"github.com/spf13/cobra"
)

func newSomaStacksCmd() *cobra.Command {
	cmd := newGroupCmd("stacks", "Where the compose files and litellm_config.yaml live (~/.dendrite/stacks)", "")
	cmd.Example = "dendrite soma stacks path\n" +
		"dendrite soma stacks migrate --dry-run\n" +
		"dendrite soma stacks migrate --up"
	cmd.AddCommand(newSomaStacksPathCmd(), newSomaStacksMigrateCmd())
	return cmd
}

func newSomaStacksPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the stacks directory",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), stacks.Dir())
			return err
		},
	}
}

func newSomaStacksMigrateCmd() *cobra.Command {
	var from string
	var dryRun, up bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Copy a checkout's stack files into ~/.dendrite/stacks (never overwrites)",
		Long: "The compose files and litellm_config.yaml live in $DENDRITE_HOME/stacks, next to the credentials in\n" +
			"$DENDRITE_HOME/.env, so every command works from any directory. This copies the maintained files of a checkout's\n" +
			"stacks/ folder there. Files already present at the destination are kept as they are, so re-running is safe.\n\n" +
			"Credentials and templates (.env*) are never copied. With --up the control stack is then started from the new\n" +
			"location (docker compose up -d), which recreates the containers whose mounts moved; named volumes (Postgres\n" +
			"data, Copilot login) are preserved.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			src := from
			if src == "" {
				src = stacks.CheckoutStacks(dotenv.ProjectRoot("."))
			}
			if src == "" {
				return usageErrorf("no checkout stacks/ folder found here; pass --from <dir>")
			}
			return runStacksMigrate(cmd.Context(), cmd.OutOrStdout(), src, dryRun, up)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Source stacks folder (default: this checkout's stacks/)")
	addDryRunFlag(cmd, &dryRun)
	cmd.Flags().BoolVar(&up, "up", false, "Then start the control stack from the new location (recreates containers whose mounts moved)")
	return cmd
}

func runStacksMigrate(ctx context.Context, out io.Writer, src string, dryRun, up bool) error {
	dst := stacks.Dir()
	rep, err := stacks.Migrate(src, dst, dryRun)
	if err != nil {
		return err
	}
	if globals.JSON {
		return writeJSON(out, rep)
	}
	verb := "Copied"
	if dryRun {
		verb = "Would copy"
	}
	fmt.Fprintf(out, "[✓] %s %d file(s) from %s to %s (%d already there, kept)\n", verb, len(rep.Copied), src, dst, len(rep.Kept))
	for _, f := range rep.Copied {
		fmt.Fprintf(out, "    + %s\n", f)
	}
	if dryRun || !up {
		if !dryRun && len(rep.Copied) > 0 {
			fmt.Fprintln(out, "    Run `dendrite soma stacks migrate --up` to start the stack from the new location.")
		}
		return nil
	}
	compose := stacks.Path("compose.control.yaml")
	if _, err := os.Stat(compose); err != nil {
		return fmt.Errorf("control compose file missing at %s: %w", compose, err)
	}
	fmt.Fprintf(out, "[+] docker compose -f %s up -d\n", compose)
	return runCompose(ctx, out, compose, "up", "-d")
}

// adoptCheckoutStacks gives a machine that has not moved its stack files yet a one-time, non-destructive
// copy of this checkout's stacks/ into ~/.dendrite/stacks, the first time a command needs them.
func adoptCheckoutStacks(out io.Writer) {
	dst := stacks.Dir()
	if _, err := os.Stat(dst); err == nil {
		return
	}
	src := stacks.CheckoutStacks(dotenv.ProjectRoot("."))
	if src == "" || filepath.Clean(src) == filepath.Clean(dst) {
		return
	}
	if rep, err := stacks.Migrate(src, dst, false); err == nil && len(rep.Copied) > 0 {
		fmt.Fprintf(out, "[+] Copied %d stack file(s) from %s to %s (existing files kept). The containers keep running until you run `dendrite soma stacks migrate --up`.\n",
			len(rep.Copied), strings.TrimSuffix(src, "/"), dst)
	}
}
