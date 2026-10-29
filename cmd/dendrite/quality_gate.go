package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/quality/gate"
	"Dendrite/pkg/rag"
)

// gateSidecar is the sidecar the gate runs its quality methods on; a seam for tests.
var gateSidecar = func() gate.Sidecar { return rag.NewRetriever() }

type gateOptions struct {
	Path, Format, Since, Ticket string
	Only, Skip                  []string
}

func newAllostasisGateCmd() *cobra.Command {
	o := gateOptions{}
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "Run the project's quality gates: production lines per file, repeated code, Rust test structure and coverage",
		Long: "Runs the gates of " + directive.Path + " as one check and exits non-zero when one fails, or could not run.\n" +
			"loc, clones and rstest run on the dendrite-rag sidecar (`dendrite update` installs it); coverage runs `go test`.\n" +
			"Without a scope the whole repository is held to the directives. --since <ref> or --ticket <ID> judges only what that\n" +
			"change touched: files it changed, repeated blocks that include lines it added, and, for coverage, the ratchet of\n" +
			"the packages it changed when the project keeps a baseline (the floor otherwise).",
		Example: "dendrite allostasis gate\ndendrite allostasis gate --since origin/develop --format markdown\ndendrite allostasis gate --only loc,clones",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if globals.JSON {
				o.Format = "json"
			}
			return runGate(cmd.Context(), o, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Path, "path", ".", "Project directory")
	f.StringVar(&o.Format, "format", "text", "text, json or markdown")
	f.StringVar(&o.Since, "since", "", "Judge only what changed since this ref (for example origin/develop)")
	f.StringVar(&o.Ticket, "ticket", "", "Judge only the work done for this ticket")
	f.StringSliceVar(&o.Only, "only", nil, "Run only these checks: "+fmt.Sprint(gate.Names))
	f.StringSliceVar(&o.Skip, "skip", nil, "Do not run these checks")
	cmd.MarkFlagsMutuallyExclusive("since", "ticket")
	return cmd
}

func runGate(ctx context.Context, o gateOptions, out io.Writer) error {
	if err := gate.Validate(append(append([]string{}, o.Only...), o.Skip...)); err != nil {
		return usageErrorf("%v", err)
	}
	if o.Format != "text" && o.Format != "json" && o.Format != "markdown" {
		return usageErrorf("--format must be text, json or markdown, not %q", o.Format)
	}
	dir, err := filepath.Abs(o.Path)
	if err != nil {
		return err
	}
	set, err := directive.Load(dir)
	if err != nil {
		return err
	}
	opts := gate.Options{Dir: dir, Set: set, Sidecar: gateSidecar(), Only: o.Only, Skip: o.Skip}
	switch {
	case o.Ticket != "":
		opts.Scope = gate.ChangesOfTicket(dir, o.Ticket)
	case o.Since != "":
		opts.Scope = gate.ChangesSince(dir, o.Since)
	}
	report := gate.Run(ctx, opts)
	switch o.Format {
	case "json":
		fmt.Fprintln(out, report.JSON())
	case "markdown":
		fmt.Fprint(out, report.Markdown())
	default:
		fmt.Fprint(out, report.Text())
	}
	if !report.Passed {
		return errors.New("the quality gate failed")
	}
	return nil
}
