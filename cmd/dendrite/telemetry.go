package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"time"

	"Dendrite/pkg/telemetry"
	"Dendrite/pkg/telemetry/exporter"

	"github.com/spf13/cobra"
)

func newTelemetryDaemon() *telemetry.Daemon {
	return telemetry.NewDaemon(telemetry.DaemonConfig{Interval: time.Second}, nil, nil, nil, nil)
}

// newSensoryCmd builds `dendrite sensory` (alias telemetry). With no subcommand it prints a snapshot.
func newSensoryCmd() *cobra.Command {
	cmd := newGroupCmd("sensory", "Hardware thermal, token spend, and Prometheus telemetry", "cortex",
		"telemetry", "afferent-telemetry", "tel")
	cmd.Example = "dendrite sensory\ndendrite sensory snapshot --json\ndendrite sensory metrics"
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			return runSensorySnapshot(c.Context(), c.OutOrStdout())
		}
		return runGroup(c, args)
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "snapshot",
			Short: "Print one telemetry snapshot (temperature, spend, node health)",
			Args:  noArgs,
			RunE: func(c *cobra.Command, args []string) error {
				return runSensorySnapshot(c.Context(), c.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:     "metrics",
			Aliases: []string{"export", "prometheus"},
			Short:   "Print the metrics in Prometheus exposition format",
			Args:    noArgs,
			RunE: func(c *cobra.Command, args []string) error {
				if err := exporter.NewPrometheusExporter(newTelemetryDaemon()).WriteMetrics(c.Context(), c.OutOrStdout()); err != nil {
					return fmt.Errorf("write Prometheus metrics: %w", err)
				}
				return nil
			},
		},
	)
	return cmd
}

func runSensorySnapshot(ctx context.Context, out io.Writer) error {
	rep := newTelemetryDaemon().CollectOnce(ctx)
	if globals.JSON {
		return writeJSON(out, rep)
	}
	fmt.Fprintln(out, "[+] Instantaneous Telemetry Snapshot:")
	fmt.Fprintf(out, "    - Timestamp:        %s\n", rep.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(out, "    - SoC Temperature:  %.1f°C (Alert: %v, Critical: %v)\n", rep.SoCTempC, rep.ThermalAlert, rep.ThermalCritical)
	fmt.Fprintf(out, "    - 24h Spend:        $%.2f / $%.2f daily budget\n", rep.Spend24hUSD, rep.DailyBudgetUSD)
	fmt.Fprintf(out, "    - 7d Spend:         $%.2f\n", rep.Spend7dUSD)
	fmt.Fprintln(out, "    - Node Health:")
	for _, node := range slices.Sorted(maps.Keys(rep.NodeStatus)) {
		fmt.Fprintf(out, "        * %-10s -> %s\n", node, rep.NodeStatus[node])
	}
	return nil
}
