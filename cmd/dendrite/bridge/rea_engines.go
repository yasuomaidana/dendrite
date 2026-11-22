package bridge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/directive"
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/rea/engine"
	"Dendrite/pkg/rea/ownership"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// addEngineFlags adds the flags that choose the reverse-engineering engine and where it runs.
func addEngineFlags(f *pflag.FlagSet, o *analysis.Options) {
	f.StringVar(&o.Engine, "engine", "", "Engine to run over the target: auto (the best usable open-source one), none, rizin, radare2 or objdump (default: the [rea] directive)")
	f.StringVar(&o.Sandbox, "sandbox", "", "Where the engine runs: container (default) or host")
	f.BoolVar(&o.Trusted, "trusted", false, "Say the target is safe to open on the host (needed for a binary with --sandbox host)")
}

// engineDefaults fills the engine and sandbox a new analysis was not told: the environment, then the [rea] directive.
func engineDefaults(o analysis.Options) analysis.Options {
	set, err := ownership.LoadRule(".")
	if err != nil {
		set = directive.Defaults()
	}
	return o.WithEngineDefaults(set.Rea)
}

// engineCommands is `dendrite retro engines` and its build subcommand.
func engineCommands() []*cobra.Command {
	engines := &cobra.Command{
		Use:   "engines",
		Short: "List the reverse-engineering engines and where each can run on this machine",
		Long: "Lists the engines Dendrite knows: the open-source ones it can run over a target (rizin, radare2, objdump, Ghidra) and\n" +
			"the proprietary tools it only documents (Hopper, IDA, Binary Ninja), which are never chosen automatically. For each:\n" +
			"license, capabilities, and whether it is usable in the container (Docker answers and its image is built) or on the\n" +
			"host (its binary is on PATH). `--engine auto` takes the best usable open-source engine. Binaries run in the\n" +
			"container unless you say --sandbox host --trusted.",
		Example: "dendrite retro engines\ndendrite retro engines build rizin",
		Args:    cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return writeEngines(cmd.OutOrStdout(), engine.Statuses(cmd.Context(), engine.NewService().Env))
		},
	}
	var opts engine.BuildOptions
	build := &cobra.Command{
		Use:   "build <engine>",
		Short: "Build the container image of an engine from its Dockerfile (never done implicitly)",
		Long: "Builds the image of rizin, radare2, objdump or ghidra from the Dockerfile embedded in dendrite. Nothing builds or\n" +
			"pulls an image by itself. Ghidra's release is large and not pinned by Dendrite: pass the https link of the release\n" +
			"zip you chose and its sha256; the build refuses a download that does not hash to it.",
		Example: "dendrite retro engines build rizin\ndendrite retro engines build ghidra --url https://github.com/NationalSecurityAgency/ghidra/releases/download/<tag>/<zip> --sha256 <64 hex digits>",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := engine.NewService().Build(cmd.Context(), args[0], opts)
			if err != nil {
				return fmt.Errorf("%w\n%s", err, strings.TrimSpace(out))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[✓] Built the image of %s\n", args[0])
			return nil
		},
	}
	build.Flags().StringVar(&opts.URL, "url", "", "Ghidra only: https link of the Ghidra release zip to install")
	build.Flags().StringVar(&opts.SHA256, "sha256", "", "Ghidra only: the checksum the release zip must have")
	engines.AddCommand(build)
	return []*cobra.Command{engines}
}

// writeEngines prints the statuses as JSON or a table.
func writeEngines(out io.Writer, statuses []engine.Status) error {
	if cli.Globals.JSON {
		return cli.WriteJSON(out, statuses)
	}
	fmt.Fprintf(out, "%-9s %-17s %-28s %-9s %s\n", "ENGINE", "LICENSE", "CAPABILITIES", "CONTAINER", "HOST")
	for _, s := range statuses {
		fmt.Fprintf(out, "%-9s %-17s %-28s %-9s %s\n", s.Spec.Name, s.Spec.License, capabilities(s.Spec.Caps), word(s.Container), word(s.Host))
		if !s.Container.Usable && s.Container.Fix != "" {
			fmt.Fprintf(out, "          container: %s (%s)\n", s.Container.Reason, s.Container.Fix)
		}
		if s.Spec.Note != "" {
			fmt.Fprintf(out, "          %s\n", s.Spec.Note)
		}
	}
	return nil
}

func word(a engine.Availability) string {
	if a.Usable {
		return "ready"
	}
	return "no"
}

func capabilities(c engine.Caps) string {
	var out []string
	for _, f := range []struct {
		on   bool
		name string
	}{{c.Functions, "functions"}, {c.Disassembly, "disassembly"}, {c.CallGraph, "graph"}, {c.Decompile, "decompile"}} {
		if f.on {
			out = append(out, f.name)
		}
	}
	return strings.Join(out, ",")
}
