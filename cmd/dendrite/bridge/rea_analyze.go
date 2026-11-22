package bridge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/rea/analysis"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// newReaService builds the analysis service (a seam for tests that need another one).
var newReaService = analysis.NewService

// reaCommands are the analysis commands of `dendrite retro`: analyze, pause, resume, purge and search.
func reaCommands() []*cobra.Command {
	var o analysis.Options
	analyze := &cobra.Command{
		Use:   "analyze <target>",
		Short: "Analyze any target (PE, ELF, Mach-O, package, bundle, source directory): inspect it, write its structure as documents and index them for search",
		Long: "Inspects the target in the dendrite-core sidecar, whatever its format (PE, ELF, Mach-O, fat Mach-O, ASAR, ZIP/APK, a\n" +
			"JS bundle, an unknown file or a directory of source), writes its sections, exports, files, strings and one document per\n" +
			"dependency, and indexes them with the RAG sidecar, saving a checkpoint after every stage. The answer is a\n" +
			"bounded evidence bundle (under 4,000 tokens) and an analysis id; read more with `dendrite retro search <id> \"...\"`\n" +
			"instead of dumping the image again. `pause` stops it at the next stage and `resume` continues it, on another\n" +
			"model tier if you like.",
		Example: "dendrite retro analyze sample.exe --tier local\ndendrite retro analyze ./app.asar\ndendrite retro analyze ./src --format source-tree\ndendrite retro search pe-1a2b3c4d \"CreateFileW\"",
		Args:    cli.ExactArgs(1),
		RunE: runStages(func(ctx context.Context, s *analysis.Service, args []string) (*analysis.Result, error) {
			return s.Analyze(ctx, args[0], o)
		}),
	}
	f := analyze.Flags()
	f.StringVar(&o.Format, "format", "", "Format the target must have (default: detect it): pe, elf, macho, fat-macho, asar, zip, js-bundle, source-tree or unknown")
	f.StringVar(&o.ID, "id", "", "Name of the analysis (default: generated from the binary)")
	f.StringVar(&o.Tier, "tier", "local", "Model tier: local or paid")
	f.StringVar(&o.Model, "model", "", "Model to use on that tier")

	pause := &cobra.Command{
		Use:   "pause <id>",
		Short: "Pause an analysis at its next stage boundary; its checkpoint is kept",
		Args:  cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withReaService(func(s *analysis.Service) error {
				st, err := s.Pause(args[0])
				if err != nil {
					return err
				}
				return writeState(cmd.OutOrStdout(), st, "[✓] %s will pause after stage %s: resume it with `dendrite retro resume %s`\n", st.ID, st.Stage, st.ID)
			})
		},
	}

	var ro analysis.Options
	resume := &cobra.Command{
		Use:     "resume <id>",
		Short:   "Continue an analysis from its last checkpoint, optionally on another model tier or model",
		Example: "dendrite retro resume pe-1a2b3c4d --tier paid --model cloud-pro",
		Args:    cli.ExactArgs(1),
		RunE: runStages(func(ctx context.Context, s *analysis.Service, args []string) (*analysis.Result, error) {
			return s.Resume(ctx, args[0], ro)
		}),
	}
	resume.Flags().StringVar(&ro.Tier, "tier", "", "Switch to this model tier: local or paid")
	resume.Flags().StringVar(&ro.Model, "model", "", "Switch to this model")

	var force bool
	purge := &cobra.Command{
		Use:   "purge <id>",
		Short: "Delete an analysis: its documents, its index and its checkpoint",
		Args:  cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withReaService(func(s *analysis.Service) error {
				if err := s.Purge(cmd.Context(), args[0], force); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] Analysis %s deleted\n", args[0])
				return nil
			})
		},
	}
	purge.Flags().BoolVar(&force, "force", false, "Confirm the deletion")

	var k int
	search := &cobra.Command{
		Use:     "search <id> <query>",
		Short:   "Search the documents of a finished analysis; only the best passages are returned",
		Example: "dendrite retro search pe-1a2b3c4d \"CreateFileW\" -k 3",
		Args:    cli.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withReaService(func(s *analysis.Service) error {
				hits, err := s.Search(cmd.Context(), args[0], args[1], k)
				if err != nil {
					return err
				}
				return writeHits(cmd.OutOrStdout(), hits)
			})
		},
	}
	search.Flags().IntVarP(&k, "top", "k", 5, "How many passages to return (at most 20)")
	return []*cobra.Command{analyze, pause, resume, purge, search}
}

// runStages is the RunE of the commands that run an analysis (analyze, resume): run it, then report how far it got.
func runStages(run func(context.Context, *analysis.Service, []string) (*analysis.Result, error)) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return withReaService(func(s *analysis.Service) error {
			res, err := run(cmd.Context(), s, args)
			return reportRun(cmd.OutOrStdout(), res, err)
		})
	}
}

// withReaService runs fn with the analysis service.
func withReaService(fn func(*analysis.Service) error) error {
	s, err := newReaService()
	if err != nil {
		return err
	}
	return fn(s)
}

// reportRun prints what an analyze or resume did. A pause is an outcome the user asked for, not a failure.
func reportRun(out io.Writer, res *analysis.Result, err error) error {
	if errors.Is(err, analysis.ErrPaused) {
		return writeState(out, res.State, "[i] Analysis %s is paused after stage %s: resume it with `dendrite retro resume %s`\n", res.State.ID, res.State.Stage, res.State.ID)
	}
	if err != nil {
		return err
	}
	if cli.Globals.JSON {
		return cli.WriteJSON(out, res)
	}
	st, b := res.State, res.Bundle
	fmt.Fprintf(out, "[✓] Analysis %s: %s (%s, tier %s)\n", st.ID, st.Stage, st.Format, st.Tier)
	fmt.Fprintf(out, "    Evidence: %d tokens (truncated: %t); imported DLLs: %s\n", b.TokenEstimate, b.Truncated, strings.Join(b.CrossReferences, ", "))
	fmt.Fprintf(out, "    Read more: dendrite retro search %s \"<question>\"\n", st.ID)
	return nil
}

// writeState prints a state as JSON, or the formatted line.
func writeState(out io.Writer, st analysis.State, format string, args ...any) error {
	if cli.Globals.JSON {
		return cli.WriteJSON(out, st)
	}
	fmt.Fprintf(out, format, args...)
	return nil
}

func writeHits(out io.Writer, hits []analysis.Hit) error {
	if cli.Globals.JSON {
		return cli.WriteJSON(out, hits)
	}
	if len(hits) == 0 {
		fmt.Fprintln(out, "[i] No passage matches.")
	}
	for _, h := range hits {
		fmt.Fprintf(out, "--- %s\n%s\n", h.File, strings.TrimSpace(h.Text))
	}
	return nil
}

// reaExportCommand is `dendrite retro export`: the structure of a finished analysis as source code in another language.
func reaExportCommand() *cobra.Command {
	var lang, output string
	cmd := &cobra.Command{
		Use:     "export <id>",
		Short:   "Write the structure of a finished analysis as safe Rust structs under --output",
		Example: "dendrite retro export pe-1a2b3c4d --target-language rust --output ./generated",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if lang != "rust" {
				return fmt.Errorf("unsupported --target-language %q: only rust is available", lang)
			}
			return withReaService(func(s *analysis.Service) error {
				path, err := s.ExportRust(args[0], output)
				if err != nil {
					return err
				}
				if cli.Globals.JSON {
					return cli.WriteJSON(cmd.OutOrStdout(), map[string]string{"id": args[0], "language": lang, "file": path})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] Analysis %s exported as Rust: %s\n", args[0], path)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&lang, "target-language", "rust", "Language of the generated code (rust)")
	cmd.Flags().StringVar(&output, "output", "", "Directory to write the generated file into")
	return cmd
}
