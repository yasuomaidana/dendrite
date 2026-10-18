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
)

// errSyncFailed signals that at least one member reported an error; details are already printed.
var errSyncFailed = errors.New("sync finished with errors")

func printSyncUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: dendrite sync [flags]")
	fmt.Fprintln(out, "\nPushes unpushed commits and refreshes the Plane board of the current project and of")
	fmt.Fprintln(out, "every project linked to it through a synapse, then records each synapse's health.")
	fmt.Fprintln(out, "\nFlags:")
	fmt.Fprintln(out, "  --synapse <id>     Sync only the members of this synapse")
	fmt.Fprintln(out, "  --all              Sync every non-archived synapse (daemon mode)")
	fmt.Fprintln(out, "  --git-only         Only push git commits")
	fmt.Fprintln(out, "  --plane-only       Only reconcile Plane states and push DEV_STATUS")
	fmt.Fprintln(out, "  --plane <IDENT>    Plane project for a repository that is in no synapse (default: $PLANE_PROJECT_ID)")
	fmt.Fprintln(out, "  --force-plane      Push DEV_STATUS even if unchanged since the last sync")
	fmt.Fprintln(out, "  --no-fetch         Skip fetching remotes before computing ahead/behind")
	fmt.Fprintln(out, "  --dry-run          Report what would happen without pushing or writing")
	fmt.Fprintln(out, "  --json             Emit the report as JSON")
	fmt.Fprintln(out, "  --watch            Keep running, syncing every interval (combine with --all for a daemon)")
	fmt.Fprintln(out, "  --interval <dur>   Watch period, e.g. 5m (default: $DENDRITE_SYNC_INTERVAL or 10m)")
}

// syncCLI carries the CLI-only settings that are not part of synapsesync.Options.
type syncCLI struct {
	asJSON   bool
	watch    bool
	interval time.Duration
}

// parseSyncArgs turns CLI flags into synapsesync options; ok=false means usage was requested.
func parseSyncArgs(args []string) (opts synapsesync.Options, cli syncCLI, ok bool, err error) {
	opts.Fetch = true
	opts.DefaultPlaneProject = os.Getenv("PLANE_PROJECT_ID")
	cli.interval = synapsesync.IntervalFromEnv()
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--synapse" || arg == "--plane":
			if i+1 >= len(args) {
				return opts, cli, true, fmt.Errorf("flag %s requires an argument", arg)
			}
			if arg == "--synapse" {
				opts.SynapseID = args[i+1]
			} else {
				opts.DefaultPlaneProject = args[i+1]
			}
			i++
		case strings.HasPrefix(arg, "--synapse="):
			opts.SynapseID = strings.TrimPrefix(arg, "--synapse=")
		case strings.HasPrefix(arg, "--plane="):
			opts.DefaultPlaneProject = strings.TrimPrefix(arg, "--plane=")
		case arg == "--all":
			opts.All = true
		case arg == "--git-only":
			opts.GitOnly = true
		case arg == "--plane-only":
			opts.PlaneOnly = true
		case arg == "--force-plane":
			opts.ForcePlane = true
		case arg == "--no-fetch":
			opts.Fetch = false
		case arg == "--dry-run":
			opts.DryRun = true
		case arg == "--json":
			cli.asJSON = true
		case arg == "--watch":
			cli.watch = true
		case arg == "--interval" || strings.HasPrefix(arg, "--interval="):
			raw := strings.TrimPrefix(arg, "--interval=")
			if arg == "--interval" {
				if i+1 >= len(args) {
					return opts, cli, true, errors.New("flag --interval requires an argument")
				}
				raw = args[i+1]
				i++
			}
			d, perr := time.ParseDuration(raw)
			if perr != nil || d <= 0 {
				return opts, cli, true, fmt.Errorf("invalid --interval %q", raw)
			}
			cli.interval = d
		case arg == "--help" || arg == "-h":
			return opts, cli, false, nil
		default:
			return opts, cli, true, fmt.Errorf("unknown flag: %s", arg)
		}
	}
	if opts.GitOnly && opts.PlaneOnly {
		return opts, cli, true, errors.New("--git-only and --plane-only are mutually exclusive")
	}
	if cli.watch && cli.interval <= 0 {
		return opts, cli, true, errors.New("--watch needs a positive interval (DENDRITE_SYNC_INTERVAL=0 disables it)")
	}
	return opts, cli, true, nil
}

// runSyncCommand executes `dendrite sync`, writing the report to out.
func runSyncCommand(ctx context.Context, args []string, out io.Writer) error {
	opts, cli, ok, err := parseSyncArgs(args)
	if err != nil {
		return err
	}
	if !ok {
		printSyncUsage(out)
		return nil
	}
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
	fmt.Fprintln(tw, "MEMBER\tSYNAPSE\tBRANCH\tAHEAD/BEHIND\tDIRTY\tGIT\tPLANE")
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
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", m.Alias, syn, branch, ab, dirty, gitSummary(m), planeSummary(m))
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

// handleSyncCommand runs runSyncCommand against stdout and exits non-zero on failure.
func handleSyncCommand(ctx context.Context, args []string) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runSyncCommand(ctx, args, os.Stdout); err != nil {
		if !errors.Is(err, errSyncFailed) {
			fmt.Fprintf(os.Stderr, "[-] %v\n", err)
		}
		os.Exit(1)
	}
}
