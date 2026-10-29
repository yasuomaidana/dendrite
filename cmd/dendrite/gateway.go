package main

import (
	"Dendrite/pkg/memfit"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"Dendrite/pkg/gateway"
	"Dendrite/pkg/topology"

	"github.com/spf13/cobra"
)

// newCleftCmd builds `dendrite cleft` (alias gateway): LiteLLM routing table updates and config rendering.
func newCleftCmd() *cobra.Command {
	cmd := newGroupCmd("cleft", "LiteLLM token gateway rendering, dynamic routing updater & scheduler", "cleft",
		"gateway", "synaptic-cleft", "gate")
	cmd.Example = "dendrite cleft update --dry-run\n" +
		"dendrite cleft schedule --interval 12h\n" +
		"dendrite cleft render azure-burst"
	cmd.AddCommand(newCleftUpdateCmd(), newCleftScheduleCmd(), newCleftRenderCmd())
	return cmd
}

// addGatewayConfigFlags registers the flags shared by update and schedule.
func addGatewayConfigFlags(cmd *cobra.Command, opts *gateway.UpdaterOptions) {
	addFileFlag(cmd, &opts.ConfigPath, opts.ConfigPath, "LiteLLM config file to rewrite")
	cmd.Flags().BoolVar(&opts.UseContainerHost, "docker", opts.UseContainerHost, "Rewrite engine hosts for LiteLLM running in Docker (--docker=false for a host install)")
}

func newCleftUpdateCmd() *cobra.Command {
	opts := gateway.DefaultUpdaterOptions()
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"sync", "refresh"},
		Short:   "Discover live inference engines and rewrite the LiteLLM routing table",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleftUpdate(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	addGatewayConfigFlags(cmd, &opts)
	addDryRunFlag(cmd, &opts.DryRun)
	return cmd
}

func runCleftUpdate(ctx context.Context, opts gateway.UpdaterOptions, out io.Writer) error {
	if !opts.DryRun {
		adoptCheckoutStacks(out)
	}
	report, err := gateway.NewRoutingTableUpdater(opts).UpdateNow(ctx)
	if err != nil {
		return fmt.Errorf("update LiteLLM routing table: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, report)
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
	if report.MemoryBudgetBytes > 0 {
		fmt.Fprintf(out, "    * Memory Budget:           %s for local models\n", memfit.GB(report.MemoryBudgetBytes))
	}
	if len(report.TooLargeModels) > 0 {
		fmt.Fprintf(out, "    * Too Large (no route):    %s\n", strings.Join(report.TooLargeModels, ", "))
	}
	if report.ActiveLocalPrimary != "" {
		fmt.Fprintf(out, "    * Active Local Primary:    %s (Score: %d on %s)\n",
			report.ActiveLocalPrimary, report.ActiveLocalPrimaryScore, report.ActiveLocalPrimaryEngine)
	}
	fmt.Fprintf(out, "    * Total Configured Routes: %d\n", report.TotalModelsConfigured)
	if !report.DryRun {
		fmt.Fprintf(out, "    * Bytes Written:           %d bytes\n", report.BytesWritten)

		// Heal and refresh the running LiteLLM container if active so new routes/config take effect immediately
		if err := restartRunningLiteLLM(ctx); err == nil {
			fmt.Fprintln(out, "    * LiteLLM Container:       Healed & reloaded successfully")
		}
	}
	return nil
}

// restartRunningLiteLLM reloads or restarts the LiteLLM proxy container if running.
func restartRunningLiteLLM(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "restart", "dendrite-litellm")
	return cmd.Run()
}

func newCleftScheduleCmd() *cobra.Command {
	opts := gateway.DefaultUpdaterOptions()
	cmd := &cobra.Command{
		Use:     "schedule",
		Aliases: []string{"daemon", "watch"},
		Short:   "Keep the LiteLLM routing table updated on an interval (foreground; Ctrl+C stops it)",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Interval <= 0 {
				return usageErrorf("--interval must be positive")
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runCleftSchedule(ctx, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().DurationVarP(&opts.Interval, "interval", "i", opts.Interval, "Time between updates")
	addGatewayConfigFlags(cmd, &opts)
	return cmd
}

// runCleftSchedule prints one line per update until ctx is cancelled.
func runCleftSchedule(ctx context.Context, opts gateway.UpdaterOptions, out io.Writer) error {
	fmt.Fprintf(out, "[+] Starting LiteLLM Routing Table Scheduler (Interval: %v)\n", opts.Interval)
	fmt.Fprintf(out, "    * Target Configuration: %s\n", opts.ConfigPath)
	fmt.Fprintf(out, "    * Container Host Rewriting: %v\n", opts.UseContainerHost)

	repCh, stopFunc := gateway.NewRoutingTableUpdater(opts).StartDailyDaemon(ctx)
	defer stopFunc()
	fmt.Fprintln(out, "[+] Scheduler active. Press Ctrl+C to stop.")

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(out, "\n[!] Stopping LiteLLM routing scheduler...")
			return nil
		case rep, ok := <-repCh:
			if !ok {
				return nil
			}
			if rep != nil {
				fmt.Fprintf(out, "[%s] [✓] Synced %d routes (%d discovered models). Primary: %s (Score: %d on %s)\n",
					rep.Timestamp.Format("15:04:05"), rep.TotalModelsConfigured, len(rep.DiscoveredModels),
					rep.ActiveLocalPrimary, rep.ActiveLocalPrimaryScore, rep.ActiveLocalPrimaryEngine)
			}
		}
	}
}

// scenarioNames lists the topology scenarios for help text and completion.
func scenarioNames() []string {
	var names []string
	for _, s := range topology.AllScenarios() {
		names = append(names, string(s))
	}
	return names
}

// parseScenarioArg parses an optional scenario argument; an invalid name is a usage error.
func parseScenarioArg(args []string) (topology.Scenario, error) {
	name := string(topology.ScenarioLocalHybrid)
	if len(args) > 0 {
		name = args[0]
	}
	sc, err := topology.ParseScenario(name)
	if err != nil {
		return "", usageErrorf("invalid scenario %q: use %s", name, strings.Join(scenarioNames(), ", "))
	}
	return sc, nil
}

// tailscaleEndpoints reads the control and execution plane addresses, with lab defaults.
func tailscaleEndpoints() topology.TailscaleEndpoints {
	ep := topology.TailscaleEndpoints{ControlPlane: "100.84.12.10", ExecutionPlane: "100.84.12.25"}
	if v := os.Getenv("TAILSCALE_CONTROL_PLANE_IP"); v != "" {
		ep.ControlPlane = v
	}
	if v := os.Getenv("TAILSCALE_EXECUTION_PLANE_IP"); v != "" {
		ep.ExecutionPlane = v
	}
	return ep
}

func newCleftRenderCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "render [scenario]",
		Short:     "Render the LiteLLM config for a topology scenario (default local-hybrid)",
		Args:      maxArgs(1),
		ValidArgs: scenarioNames(),
		RunE: func(cmd *cobra.Command, args []string) error {
			sc, err := parseScenarioArg(args)
			if err != nil {
				return err
			}
			return runCleftRender(sc, cmd.OutOrStdout())
		},
	}
}

func runCleftRender(sc topology.Scenario, out io.Writer) error {
	top, err := topology.NewTopologyConfig(sc, tailscaleEndpoints())
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
