package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"Dendrite/pkg/oxilease"

	"github.com/spf13/cobra"
)

// newSpikeSuperviseCmd is the detached process that owns the OxiLLaMa server. Dendrite starts it
// when a session needs the server; it stops the server once no session holds a lease.
func newSpikeSuperviseCmd() *cobra.Command {
	var model, modelID, dir string
	var port int
	cmd := &cobra.Command{
		Use:    "supervise",
		Short:  "Run the OxiLLaMa server and stop it when no session needs it (started by Dendrite, not by hand)",
		Hidden: true,
		Args:   noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			m := oxilease.NewManager(port, dir)
			return m.Supervise(ctx, oxilease.Request{Holder: "supervisor", ModelPath: model, ModelID: modelID})
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "GGUF model file")
	cmd.Flags().StringVar(&modelID, "model-id", "", "id the server serves the model under")
	cmd.Flags().StringVar(&dir, "dir", "", "lease directory")
	cmd.Flags().IntVar(&port, "port", oxilease.DefaultPort, "server port")
	_ = cmd.MarkFlagRequired("model")
	_ = cmd.MarkFlagRequired("dir")
	return cmd
}

// listeners returns the pids LISTENING on the port. Clients connected to it are not included:
// stopping those would kill browsers, Docker, or other sessions.
func listeners(ctx context.Context, port int) []int {
	out, _ := exec.CommandContext(ctx, "lsof", "-ti", fmt.Sprintf("TCP:%d", port), "-sTCP:LISTEN").Output()
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(f); err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids
}

// terminate asks a process to exit and kills it if it has not after the grace.
func terminate(pid int, grace time.Duration) bool {
	proc, err := os.FindProcess(pid)
	if err != nil || proc.Signal(syscall.SIGTERM) != nil {
		return false
	}
	for deadline := time.Now().Add(grace); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if proc.Signal(syscall.Signal(0)) != nil {
			return true
		}
	}
	return proc.Kill() == nil
}

func runSpikeStop(ctx context.Context, port int, out io.Writer) error {
	fmt.Fprintf(out, "[+] Stopping the OxiLLaMa server on port %d...\n", port)
	if holders, err := oxilease.NewManager(port, "").Holders(); err == nil && len(holders) > 0 {
		fmt.Fprintf(out, "[!] %d live session(s) hold a lease and will lose it:\n", len(holders))
		for _, h := range holders {
			fmt.Fprintf(out, "    - %s (pid %d)\n", h.Holder, h.PID)
		}
	}
	stopped := 0
	for _, pid := range listeners(ctx, port) {
		if terminate(pid, 5*time.Second) {
			stopped++
		}
	}
	if stopped > 0 {
		fmt.Fprintf(out, "[✓] Successfully stopped OxiLLaMa server (terminated %d process(es) on port %d)\n", stopped, port)
	} else {
		fmt.Fprintf(out, "[✓] OxiLLaMa server is not currently running on port %d.\n", port)
	}
	return nil
}

func newSpikeStatusCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show whether the OxiLLaMa server runs, who started it and which sessions hold it",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := oxilease.NewManager(port, "").Status()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, st)
			}
			switch {
			case !st.Running:
				fmt.Fprintln(out, "[+] OxiLLaMa is not running (Dendrite starts it when a session needs it)")
			case st.Managed:
				fmt.Fprintf(out, "[+] OxiLLaMa is running (model %s, pid %d), managed by Dendrite: stops %s after the last session\n", st.Model, st.ChildPID, st.IdleGrace)
			default:
				fmt.Fprintln(out, "[+] OxiLLaMa is running but was started by hand: Dendrite will not stop it (use `dendrite spike stop`)")
			}
			for _, h := range st.Holders {
				fmt.Fprintf(out, "    - held by %s (pid %d)\n", h.Holder, h.PID)
			}
			if len(st.Holders) == 0 && st.Running && st.Managed {
				fmt.Fprintln(out, "    (no session holds it; it will stop when the idle grace ends)")
			}
			return nil
		},
	}
	addPortFlag(cmd, &port)
	return cmd
}

// newSpikeHoldCmd takes a lease like a session does, so scripts and long agent jobs can keep the
// server alive exactly as long as they run.
func newSpikeHoldCmd() *cobra.Command {
	var port int
	var dur time.Duration
	cmd := &cobra.Command{
		Use:   "hold",
		Short: "Start OxiLLaMa if needed and keep it alive while this command runs (--for, or until Ctrl+C)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			m := oxilease.NewManager(port, "")
			req := oxilease.Request{Holder: "hold"}
			if !m.Running() {
				best, err := oxilease.BestFitting(ctx)
				if err != nil {
					return err
				}
				req.ModelPath, req.ModelID = best.Path, best.ID
			}
			lease, err := m.Acquire(ctx, req)
			if err != nil {
				return err
			}
			defer func() { _ = lease.Release() }()
			fmt.Fprintf(cmd.OutOrStdout(), "[✓] Holding OxiLLaMa on port %d\n", port)
			var done <-chan time.Time
			if dur > 0 {
				done = time.After(dur)
			}
			beat := time.NewTicker(m.TTL() / 3)
			defer beat.Stop()
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-done:
					return nil
				case <-beat.C:
					_ = lease.Heartbeat()
				}
			}
		},
	}
	addPortFlag(cmd, &port)
	cmd.Flags().DurationVar(&dur, "for", 0, "hold for this long (default: until interrupted)")
	return cmd
}
