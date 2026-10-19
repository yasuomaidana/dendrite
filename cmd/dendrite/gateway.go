package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"Dendrite/pkg/gateway"
	"Dendrite/pkg/topology"
)

// newCleftCmd builds `dendrite cleft`: LiteLLM token gateway rendering, dynamic routing updater & scheduler.
func newCleftCmd() *cobra.Command {
	cmd := newGroupCmd("cleft", "LiteLLM token gateway rendering, dynamic routing updater & scheduler", "cleft",
		"gateway", "synaptic-cleft", "gate")
	cmd.Example = "dendrite cleft update --dry-run\ndendrite cleft schedule --interval 24h\ndendrite cleft render local-hybrid"
	cmd.AddCommand(
		newCleftUpdateCmd(),
		newCleftScheduleCmd(),
		newCleftRenderCmd(),
	)
	return cmd
}

type cleftUpdateOptions struct {
	File   string
	DryRun bool
	Docker bool
}

func newCleftUpdateCmd() *cobra.Command {
	defaults := gateway.DefaultUpdaterOptions()
	o := cleftUpdateOptions{
		File:   defaults.ConfigPath,
		Docker: defaults.UseContainerHost,
	}
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"sync", "refresh"},
		Short:   "Dynamically discover live inference engines and update LiteLLM routing table",
		Args:    noArgs,
		Example: "dendrite cleft update\ndendrite cleft update --file stacks/litellm_config.yaml --dry-run --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleftUpdate(cmd.Context(), o, globals.JSON, cmd.OutOrStdout())
		},
	}
	addFileFlag(cmd, &o.File, defaults.ConfigPath, "Target litellm_config.yaml path")
	addDryRunFlag(cmd, &o.DryRun)
	cmd.Flags().BoolVar(&o.Docker, "docker", defaults.UseContainerHost, "Rewrite localhost/127.0.0.1 to host.docker.internal")
	return cmd
}

func runCleftUpdate(ctx context.Context, o cleftUpdateOptions, asJSON bool, out io.Writer) error {
	opts := gateway.DefaultUpdaterOptions()
	if o.File != "" {
		opts.ConfigPath = o.File
	}
	opts.DryRun = o.DryRun
	opts.UseContainerHost = o.Docker

	updater := gateway.NewRoutingTableUpdater(opts)
	report, err := updater.UpdateNow(ctx)
	if err != nil {
		return fmt.Errorf("update LiteLLM routing table: %w", err)
	}

	if asJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("serialize JSON report: %w", err)
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	modeStr := "Updated"
	if report.DryRun {
		modeStr = "Simulated Update (Dry-Run)"
	}

	fmt.Fprintf(out, "[✓] LiteLLM Routing Table %s Successfully in %v\n", modeStr, report.Duration)
	fmt.Fprintf(out, "    * Config Path:             %s\n", report.ConfigPath)
	fmt.Fprintf(out, "    * Engines Probed:          %d (Online: %d, Offline: %d)\n",
		report.EnginesProbed, len(report.OnlineEngines), len(report.OfflineEngines))
	if len(report.OnlineEngines) > 0 {
		fmt.Fprintf(out, "    * Online Engines:          %s\n", strings.Join(report.OnlineEngines, ", "))
	}
	if len(report.OfflineEngines) > 0 {
		fmt.Fprintf(out, "    * Offline Engines:         %s\n", strings.Join(report.OfflineEngines, ", "))
	}
	fmt.Fprintf(out, "    * Discovered Local Models: %d\n", len(report.DiscoveredModels))
	if report.ActiveLocalPrimary != "" {
		fmt.Fprintf(out, "    * Active Local Primary:    %s (Score: %d on %s)\n",
			report.ActiveLocalPrimary, report.ActiveLocalPrimaryScore, report.ActiveLocalPrimaryEngine)
	}
	fmt.Fprintf(out, "    * Total Configured Routes: %d\n", report.TotalModelsConfigured)
	if !report.DryRun {
		fmt.Fprintf(out, "    * Bytes Written:           %d bytes\n", report.BytesWritten)
	}
	return nil
}

type cleftScheduleOptions struct {
	Interval time.Duration
	File     string
	Docker   bool
}

func newCleftScheduleCmd() *cobra.Command {
	defaults := gateway.DefaultUpdaterOptions()
	o := cleftScheduleOptions{
		Interval: defaults.Interval,
		File:     defaults.ConfigPath,
		Docker:   defaults.UseContainerHost,
	}
	cmd := &cobra.Command{
		Use:     "schedule",
		Aliases: []string{"daemon", "watch"},
		Short:   "Run dynamic routing table updater on a recurring background schedule",
		Args:    noArgs,
		Example: "dendrite cleft schedule --interval 24h --file stacks/litellm_config.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.Interval <= 0 {
				return usageErrorf("--interval must be greater than 0 (got %v)", o.Interval)
			}
			return runCleftSchedule(cmd.Context(), o, cmd.OutOrStdout())
		},
	}
	addFileFlag(cmd, &o.File, defaults.ConfigPath, "Target litellm_config.yaml path")
	cmd.Flags().DurationVar(&o.Interval, "interval", defaults.Interval, "Scheduled update interval (must be > 0)")
	cmd.Flags().BoolVar(&o.Docker, "docker", defaults.UseContainerHost, "Rewrite localhost/127.0.0.1 to host.docker.internal")
	return cmd
}

func runCleftSchedule(ctx context.Context, o cleftScheduleOptions, out io.Writer) error {
	opts := gateway.DefaultUpdaterOptions()
	opts.Interval = o.Interval
	if o.File != "" {
		opts.ConfigPath = o.File
	}
	opts.UseContainerHost = o.Docker

	updater := gateway.NewRoutingTableUpdater(opts)
	fmt.Fprintf(out, "[+] Starting LiteLLM Routing Table Scheduler Daemon (Interval: %v)\n", opts.Interval)
	fmt.Fprintf(out, "    * Target Configuration: %s\n", opts.ConfigPath)
	fmt.Fprintf(out, "    * Container Host Rewriting: %v\n", opts.UseContainerHost)

	daemonCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	repCh, stopFunc := updater.StartDailyDaemon(daemonCtx)
	defer stopFunc()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	fmt.Fprintln(out, "[+] Daemon active. Monitoring inference engines and keeping routes updated.")
	fmt.Fprintln(out, "    (Press Ctrl+C to terminate)")

	for {
		select {
		case <-ctx.Done():
			stopFunc()
			return nil
		case sig := <-sigCh:
			fmt.Fprintf(out, "\n[!] Received signal %v. Terminating LiteLLM routing scheduler...\n", sig)
			stopFunc()
			return nil
		case rep, ok := <-repCh:
			if !ok {
				return nil
			}
			if rep != nil {
				fmt.Fprintf(out, "[%s] [✓] Synced %d routes (%d discovered models). Primary: %s (Score: %d on %s)\n",
					rep.Timestamp.Format("15:04:05"), rep.TotalModelsConfigured, len(reportModels(rep)),
					rep.ActiveLocalPrimary, rep.ActiveLocalPrimaryScore, rep.ActiveLocalPrimaryEngine)
			}
		}
	}
}

func reportModels(rep *gateway.UpdateReport) []gateway.DiscoveredModel {
	if rep == nil {
		return nil
	}
	return rep.DiscoveredModels
}

func newCleftRenderCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "render [scenario]",
		Short:   "Render dynamic LiteLLM configuration for a topology scenario",
		Args:    maxArgs(1),
		Example: "dendrite cleft render\ndendrite cleft render local-hybrid\ndendrite cleft render distributed",
		RunE: func(cmd *cobra.Command, args []string) error {
			scenarioStr := "local-hybrid"
			if len(args) > 0 {
				scenarioStr = args[0]
			}
			return renderGatewayScenario(cmd.Context(), scenarioStr, cmd.OutOrStdout())
		},
	}
}

func renderGatewayScenario(_ context.Context, scenarioStr string, out io.Writer) error {
	sc, err := topology.ParseScenario(scenarioStr)
	if err != nil {
		return usageErrorf("invalid scenario %q: %w", scenarioStr, err)
	}

	ctrlIP := os.Getenv("TAILSCALE_CONTROL_PLANE_IP")
	if ctrlIP == "" {
		ctrlIP = "100.84.12.10"
	}
	execIP := os.Getenv("TAILSCALE_EXECUTION_PLANE_IP")
	if execIP == "" {
		execIP = "100.84.12.25"
	}

	top, err := topology.NewTopologyConfig(sc, topology.TailscaleEndpoints{
		ControlPlane:   ctrlIP,
		ExecutionPlane: execIP,
	})
	if err != nil {
		return fmt.Errorf("create topology: %w", err)
	}
	yamlData, err := gateway.RenderForTopology(top)
	if err != nil {
		return fmt.Errorf("render LiteLLM gateway config: %w", err)
	}
	fmt.Fprintf(out, "[+] Rendered LiteLLM Config for scenario '%s':\n\n%s\n", sc, string(yamlData))
	return nil
}

func handleGatewayCommand(ctx context.Context, args []string) {
	_ = invokeCmd(ctx, newCleftCmd(), args...)
}

