package bridge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/internal/planecli"
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/rea/insight"
	"Dendrite/pkg/tracker/plane"
	"context"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"strings"
)

// newReaService builds the analysis service (a seam for tests that need another one).
var newReaService = analysis.NewService

// reaCommands are the analysis commands of `dendrite retro`: analyze, pause, resume, purge and search.
func reaCommands() []*cobra.Command {
	var o analysis.Options
	var ticket string
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
			return s.Analyze(ctx, args[0], engineDefaults(o))
		}, func(cmd *cobra.Command, res *analysis.Result) {
			if ticket != "" {
				postTicketNote(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), ticket, res)
			}
		}),
	}
	f := analyze.Flags()
	f.StringVar(&ticket, "ticket", "", "Leave a note with the summary and the analysis id on this ticket")
	f.StringVar(&o.Format, "format", "", "Format the target must have (default: detect it): pe, elf, macho, fat-macho, asar, zip, js-bundle, source-tree or unknown")
	f.StringVar(&o.ID, "id", "", "Name of the analysis (default: generated from the binary)")
	f.StringVar(&o.Tier, "tier", "local", "Model tier: local or paid")
	f.StringVar(&o.Model, "model", "", "Model to use on that tier")
	addEngineFlags(f, &o)

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
		}, nil),
	}
	resume.Flags().StringVar(&ro.Tier, "tier", "", "Switch to this model tier: local or paid")
	resume.Flags().StringVar(&ro.Model, "model", "", "Switch to this model")
	addEngineFlags(resume.Flags(), &ro)

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
func runStages(run func(context.Context, *analysis.Service, []string) (*analysis.Result, error), after func(*cobra.Command, *analysis.Result)) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return withReaService(func(s *analysis.Service) error {
			res, err := run(cmd.Context(), s, args)
			if rerr := reportRun(cmd.OutOrStdout(), res, err); rerr != nil || err != nil || after == nil {
				return rerr
			}
			after(cmd, res)
			return nil
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
	if errors.Is(err, analysis.ErrPaused) && res.State.Gate != "" {
		return writeState(out, res.State, "[i] Analysis %s stopped at the pause gate: %s\n    Continue on the local tier with `dendrite retro resume %s`, or allow the paid model with `dendrite retro resume %s --tier paid`\n", res.State.ID, res.State.Gate, res.State.ID, res.State.ID)
	}
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
	fmt.Fprintf(out, "    Engine: %s\n", engineLine(st))
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

// insightCommands are the commands that put a finished analysis to use: reuse, rank and adr.
func insightCommands() []*cobra.Command {
	var ro insight.ReuseOptions
	reuse := &cobra.Command{
		Use:   "reuse <id>",
		Short: "Find code in this project and its synapse that already does what the analysis found",
		Long: "Asks the project (and the synapse it belongs to, or the one named with --synapse) about the exports, readable\n" +
			"strings and non-system dependencies of an analysis, and returns the best matching passages, ranked and under\n" +
			"4,000 tokens, so a cleanroom recreation starts from what the ecosystem already has.",
		Example: "dendrite retro reuse pe-1a2b3c4d\ndendrite retro reuse pe-1a2b3c4d --synapse dendrite-ecosystem -k 5",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withReaService(func(s *analysis.Service) error {
				hits, err := insight.Reuse(cmd.Context(), s, args[0], ro)
				if err != nil {
					return err
				}
				return writeReuse(cmd.OutOrStdout(), hits)
			})
		},
	}
	reuse.Flags().StringVar(&ro.Project, "project", ".", "Project directory to search")
	reuse.Flags().StringVar(&ro.Synapse, "synapse", "", "Synapse to search (default: the one the project belongs to)")
	reuse.Flags().IntVarP(&ro.K, "top", "k", 10, "How many passages to return")

	rank := &cobra.Command{
		Use:   "rank <id>",
		Short: "Say which documents of an analysis to read first (most connected first)",
		Args:  cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withReaService(func(s *analysis.Service) error {
				st, img, err := s.Inspected(args[0])
				if err != nil {
					return err
				}
				return writeRank(cmd.OutOrStdout(), insight.Rank(st, img))
			})
		},
	}

	var title, output string
	adr := &cobra.Command{
		Use:   "adr <id>",
		Short: "Draft a proposed Architecture Decision Record from a finished analysis",
		Long: "Writes docs/architecture/decisions/ADR-NNN.md (the next free number, status proposed) with the evidence bundle as\n" +
			"its context. The decision and the consequences are left for a person: agents may not implement before the record\n" +
			"is signed off (FEAT-003).",
		Example: "dendrite retro adr pe-1a2b3c4d --title \"Recreate the checksum routine\"",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withReaService(func(s *analysis.Service) error {
				path, err := insight.DraftADR(s, args[0], output, title)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] Drafted %s (status proposed): a person completes the decision and consequences\n", path)
				return nil
			})
		},
	}
	adr.Flags().StringVar(&title, "title", "", "Title of the record (default: Cleanroom recreation of <target>)")
	adr.Flags().StringVarP(&output, "output", "o", insight.DefaultADRDir, "Directory of the project's decision records")
	return []*cobra.Command{reuse, rank, adr}
}

func writeReuse(out io.Writer, hits []insight.ReuseHit) error {
	if cli.Globals.JSON {
		return cli.WriteJSON(out, hits)
	}
	if len(hits) == 0 {
		fmt.Fprintln(out, "[i] Nothing in the project or its synapse matches what this analysis found.")
	}
	for _, h := range hits {
		fmt.Fprintf(out, "--- [%s] %s (score %.2f, found by %s)\n%s\n", h.Source, h.File, h.Score, strings.Join(h.Queries, ", "), strings.TrimSpace(h.Text))
	}
	return nil
}

func writeRank(out io.Writer, ranked []insight.Ranked) error {
	if cli.Globals.JSON {
		return cli.WriteJSON(out, ranked)
	}
	for i, r := range ranked {
		fmt.Fprintf(out, "%2d. %s: %s\n", i+1, r.Doc, r.Why)
	}
	return nil
}

// postTicketNote leaves the summary of a finished analysis on the ticket. A failure is reported and goes no further:
// the analysis itself succeeded.
func postTicketNote(ctx context.Context, out, warn io.Writer, ticket string, res *analysis.Result) {
	err := planecli.PlaneRun(func(c *plane.Client) error {
		return insight.PostNote(ctx, planecli.LifecycleIn(c, "."), ticket, res)
	})
	if err != nil {
		fmt.Fprintf(warn, "[!] The analysis finished but its note was not posted on %s: %v\n", ticket, err)
		return
	}
	fmt.Fprintf(out, "    Note posted on %s\n", ticket)
}

// engineLine says which engine ran and where, or why none did.
func engineLine(st analysis.State) string {
	if st.EngineRan != "" {
		return fmt.Sprintf("%s (%s)", st.EngineRan, st.EngineMode)
	}
	if st.EngineNote != "" {
		return "none: " + st.EngineNote
	}
	return "not run"
}
