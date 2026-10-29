package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"Dendrite/pkg/agentjobs"

	"github.com/spf13/cobra"
)

func newJobsWaitCmd() *cobra.Command {
	var ticket string
	timeout, interval := 30*time.Minute, 2*time.Second
	cmd := &cobra.Command{
		Use:   "wait",
		Short: "Block until the matching jobs finish",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if timeout <= 0 || interval <= 0 {
				return usageErrorf("--timeout and --interval must be positive")
			}
			return runJobsWait(cmd.Context(), agentjobs.NewLedger(""), ticket, timeout, interval, cmd.OutOrStdout())
		},
	}
	addTicketFlag(cmd, &ticket, "")
	cmd.Flags().DurationVar(&timeout, "timeout", timeout, "Give up after this long")
	cmd.Flags().DurationVar(&interval, "interval", interval, "Polling interval")
	return cmd
}

// runJobsWait returns once every matching job has a done marker newer than when waiting began.
func runJobsWait(ctx context.Context, l *agentjobs.Ledger, ticket string, timeout, interval time.Duration, out io.Writer) error {
	var targets []*agentjobs.Job
	if ticket != "" {
		resolvedLedger, matches, err := agentjobs.ResolveTicketLedger(l, ticket)
		if err != nil || len(matches) == 0 {
			return fmt.Errorf("no matching jobs found for ticket %s", ticket)
		}
		l = resolvedLedger
		targets = matches
	} else {
		all, err := l.List()
		if err != nil {
			return fmt.Errorf("list jobs: %w", err)
		}
		targets = all
		if len(targets) == 0 {
			fmt.Fprintln(out, "[-] No matching jobs found")
			return nil
		}
	}

	initialDone := make(map[string]time.Time)
	for _, j := range targets {
		if fi, err := os.Stat(l.DoneMarkerPath(j.ID)); err == nil {
			initialDone[j.ID] = fi.ModTime()
		}
	}
	if len(initialDone) == len(targets) {
		for _, j := range targets {
			fmt.Fprintf(out, "[✓] Job finished: %s\n", j.ID)
		}
		return nil
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	finished := make(map[string]bool)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("wait timed out after %v", timeout)
		case <-tick.C:
		}
		for _, j := range targets {
			if finished[j.ID] {
				continue
			}
			fi, err := os.Stat(l.DoneMarkerPath(j.ID))
			if err != nil {
				continue
			}
			if prev, had := initialDone[j.ID]; !had || fi.ModTime().After(prev) {
				finished[j.ID] = true
				fmt.Fprintf(out, "[✓] Job finished: %s\n", j.ID)
			}
		}
		pending := 0
		for _, j := range targets {
			if _, had := initialDone[j.ID]; !had && !finished[j.ID] {
				pending++
			}
		}
		if pending == 0 && len(finished) > 0 {
			return nil
		}
	}
}

func newJobsDaemonCmd() *cobra.Command {
	var once, install bool
	var idleExit time.Duration
	interval := agentjobs.DefaultDaemonInterval
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the stall watcher (foreground), once, or install it as a LaunchAgent",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if install {
				plist, err := agentjobs.InstallLaunchAgent("", "")
				if err != nil {
					return fmt.Errorf("install launch agent: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] LaunchAgent installed to %s\n", plist)
				return nil
			}
			w := agentjobs.NewWatcher(nil, nil)
			if once {
				return w.Tick(cmd.Context())
			}
			if interval <= 0 {
				return usageErrorf("--interval must be positive")
			}
			return runJobsDaemon(cmd.Context(), w, interval, idleExit, cmd.ErrOrStderr())
		},
	}
	cmd.Flags().DurationVar(&idleExit, "idle-exit", 0, "Exit once no job has needed the watcher for this long (0: run until stopped)")
	cmd.Flags().BoolVar(&once, "once", false, "Run one watcher pass and exit")
	cmd.Flags().BoolVar(&install, "install", false, "Install the watcher as a macOS LaunchAgent")
	cmd.Flags().DurationVar(&interval, "interval", interval, "Time between watcher passes")
	cmd.MarkFlagsMutuallyExclusive("once", "install")
	return cmd
}

// runJobsDaemon ticks the watcher until ctx is cancelled, or until no job has needed it for idleExit
// (0 never); tick errors are logged, not fatal. A second watcher over the same ledger does not start.
func runJobsDaemon(ctx context.Context, w *agentjobs.Watcher, interval, idleExit time.Duration, errOut io.Writer) error {
	if pid, alive := w.Ledger.WatcherAlive(time.Now()); alive && pid != os.Getpid() {
		fmt.Fprintf(errOut, "[i] a stall watcher is already running over this ledger (pid %d)\n", pid)
		return nil
	}
	idleSince := time.Now()
	tick := func() bool {
		if err := w.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(errOut, "[-] daemon tick error: %v\n", err)
		}
		if w.Busy() {
			idleSince = time.Now()
		}
		return idleExit > 0 && time.Since(idleSince) >= idleExit
	}
	if tick() {
		return nil
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if tick() {
				fmt.Fprintf(errOut, "[i] no job needed the watcher for %s: exiting\n", idleExit)
				return nil
			}
		}
	}
}
