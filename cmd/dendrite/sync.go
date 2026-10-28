package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"Dendrite/pkg/synapsesync"
	"Dendrite/pkg/tracker/plane"

	"github.com/spf13/cobra"
)

// errSyncFailed signals that at least one member reported an error; details are already printed.
var errSyncFailed = errors.New("sync finished with errors")

// syncCLI carries the CLI-only settings that are not part of synapsesync.Options.
type syncCLI struct {
	asJSON   bool
	watch    bool
	interval time.Duration
}

// syncFlags are the `dendrite sync` flags.
type syncFlags struct {
	Synapse    string
	Plane      string
	All        bool
	GitOnly    bool
	PlaneOnly  bool
	NoRAG      bool
	ForcePlane bool
	NoFetch    bool
	DryRun     bool
	Watch      bool
	Interval   time.Duration
}

func (f *syncFlags) bind(cmd *cobra.Command) {
	fs := cmd.Flags()
	fs.StringVar(&f.Synapse, "synapse", "", "Sync only the members of this synapse")
	fs.BoolVar(&f.All, "all", false, "Sync every non-archived synapse (daemon mode)")
	fs.BoolVar(&f.GitOnly, "git-only", false, "Only push git commits")
	fs.BoolVar(&f.PlaneOnly, "plane-only", false, "Only reconcile Plane states and push DEV_STATUS")
	fs.BoolVar(&f.NoRAG, "no-rag", false, "Do not refresh each member's code index")
	fs.StringVar(&f.Plane, "plane", "", "Plane project for a repository that is in no synapse (default: $PLANE_PROJECT_ID)")
	fs.BoolVar(&f.ForcePlane, "force-plane", false, "Push DEV_STATUS even if unchanged since the last sync")
	fs.BoolVar(&f.NoFetch, "no-fetch", false, "Skip fetching remotes before computing ahead/behind")
	fs.BoolVar(&f.DryRun, "dry-run", false, "Report what would happen without pushing or writing")
	fs.BoolVar(&f.Watch, "watch", false, "Keep running, syncing every interval (combine with --all for a daemon)")
	fs.DurationVar(&f.Interval, "interval", 0, "Watch period, e.g. 5m (default: $DENDRITE_SYNC_INTERVAL or 10m)")
	cmd.MarkFlagsMutuallyExclusive("git-only", "plane-only")
}

// options turns the flags into synapsesync options and the CLI-only settings.
func (f syncFlags) options(intervalSet bool) (synapsesync.Options, syncCLI, error) {
	opts := synapsesync.Options{
		Fetch: !f.NoFetch, SynapseID: f.Synapse, All: f.All, GitOnly: f.GitOnly,
		PlaneOnly: f.PlaneOnly, NoRAG: f.NoRAG, ForcePlane: f.ForcePlane, DryRun: f.DryRun,
		DefaultPlaneProject: f.Plane,
	}
	if opts.DefaultPlaneProject == "" {
		opts.DefaultPlaneProject = os.Getenv("PLANE_PROJECT_ID")
	}
	cli := syncCLI{asJSON: globals.JSON, watch: f.Watch, interval: synapsesync.IntervalFromEnv()}
	if intervalSet {
		if f.Interval <= 0 {
			return opts, cli, usageErrorf("invalid --interval %s", f.Interval)
		}
		cli.interval = f.Interval
	}
	if opts.GitOnly && opts.PlaneOnly {
		return opts, cli, usageErrorf("--git-only and --plane-only are mutually exclusive")
	}
	if cli.watch && cli.interval <= 0 {
		return opts, cli, usageErrorf("--watch needs a positive interval (DENDRITE_SYNC_INTERVAL=0 disables it)")
	}
	return opts, cli, nil
}

// newSyncCmd builds `dendrite sync` (alias propagate): push commits and refresh Plane across synapse peers.
func newSyncCmd() *cobra.Command {
	var f syncFlags
	cmd := &cobra.Command{
		Use:     "sync",
		Aliases: []string{"propagate"},
		Short:   "Push commits and refresh Plane for this project and its synapse peers",
		Long: "Pushes unpushed commits and refreshes the Plane board of the current project and of\n" +
			"every project linked to it through a synapse, then records each synapse's health.",
		GroupID: "boutons",
		Args:    noArgs,
		Example: "dendrite sync\ndendrite sync --all --watch --interval 10m\ndendrite sync --synapse dendrite-ecosystem --dry-run\ndendrite sync --all --git-only",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, cli, err := f.options(cmd.Flags().Changed("interval"))
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runSync(ctx, opts, cli, cmd.OutOrStdout())
		},
	}
	f.bind(cmd)
	return cmd
}

// runSync executes `dendrite sync`, writing the report to out.
func runSync(ctx context.Context, opts synapsesync.Options, cli syncCLI, out io.Writer) error {
	var err error
	if !opts.All {
		if opts.StartDir, err = os.Getwd(); err != nil {
			return err
		}
	}
	if !opts.GitOnly {
		if client, cerr := plane.NewClientFromEnv(); cerr == nil {
			opts.Plane = client
		}
	}

	if cli.watch {
		fmt.Fprintf(out, "[+] Watching: syncing every %s (Ctrl-C to stop)\n", cli.interval)
		return synapsesync.Watch(ctx, opts, cli.interval, func(report *synapsesync.Report, err error) {
			fmt.Fprintf(out, "\n[%s] sync pass\n", time.Now().Format(time.RFC3339))
			if err != nil {
				fmt.Fprintf(out, "[-] %v\n", err)
				return
			}
			_ = emitSyncReport(out, report, cli.asJSON)
		})
	}

	report, err := synapsesync.Run(ctx, opts)
	if err != nil {
		return err
	}
	if err := emitSyncReport(out, report, cli.asJSON); err != nil {
		return err
	}
	if report.Failed() {
		return errSyncFailed
	}
	return nil
}

func emitSyncReport(out io.Writer, report *synapsesync.Report, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	printSyncReport(out, report)
	return nil
}

func printSyncReport(out io.Writer, r *synapsesync.Report) {
	if r.DryRun {
		fmt.Fprintln(out, "[i] Dry run: nothing was pushed or written")
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "MEMBER\tSYNAPSE\tBRANCH\tAHEAD/BEHIND\tDIRTY\tGIT\tPLANE\tINDEX")
	for _, m := range r.Members {
		branch, ab, dirty := "-", "-", "-"
		if m.Repo != nil {
			branch = m.Repo.Branch
			ab = fmt.Sprintf("%d/%d", m.Repo.Ahead, m.Repo.Behind)
			dirty = fmt.Sprintf("%d", len(m.Repo.Dirty))
		}
		syn := m.Synapse
		if syn == "" {
			syn = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", m.Alias, syn, branch, ab, dirty, gitSummary(m), planeSummary(m), ragSummary(m))
	}
	_ = tw.Flush()

	for _, m := range r.Members {
		for _, e := range m.Errors {
			fmt.Fprintf(out, "[-] %s: %s\n", m.Alias, e)
		}
		if m.Repo != nil && len(m.Repo.Dirty) > 0 {
			fmt.Fprintf(out, "[!] %s has %d uncommitted path(s); run `dendrite commit --synapse` to commit them\n", m.Alias, len(m.Repo.Dirty))
		}
	}
	for _, s := range r.Synapses {
		if s.Before != s.After {
			fmt.Fprintf(out, "[~] synapse %s: %s -> %s\n", s.ID, s.Before, s.After)
		} else {
			fmt.Fprintf(out, "[✓] synapse %s: %s\n", s.ID, s.After)
		}
	}
}

func gitSummary(m synapsesync.MemberReport) string {
	switch {
	case m.Push == nil && m.Repo == nil:
		return "-"
	case m.Push == nil:
		return "up to date"
	case m.Push.Pushed:
		return "pushed " + m.Push.Remote + "/" + m.Push.Branch
	case m.Push.Err != nil:
		return "push failed"
	default:
		return "skipped: " + m.Push.Reason
	}
}

func planeSummary(m synapsesync.MemberReport) string {
	p := m.Plane
	if p == nil {
		return "-"
	}
	parts := []string{}
	if p.Project != "" {
		parts = append(parts, p.Project)
	}
	if p.StateChanges > 0 {
		parts = append(parts, fmt.Sprintf("%d state fix(es)", p.StateChanges))
	}
	if p.StatusPushed {
		parts = append(parts, fmt.Sprintf("%d ticket(s) pushed", p.Tickets))
	} else if p.Skipped != "" {
		parts = append(parts, p.Skipped)
	}
	return strings.Join(parts, ": ")
}

// ragSummary is the INDEX column: what the code-index refresh did for a member.
func ragSummary(m synapsesync.MemberReport) string {
	r := m.RAG
	switch {
	case r == nil:
		return "-"
	case r.Indexed && r.EmbedPending > 0:
		return fmt.Sprintf("%d file(s) indexed, %d chunks to embed", r.FilesChanged, r.EmbedPending)
	case r.Indexed:
		return fmt.Sprintf("%d file(s) indexed", r.FilesChanged)
	default:
		return r.Skipped
	}
}
