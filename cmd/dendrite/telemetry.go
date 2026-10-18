package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"Dendrite/pkg/telemetry"
	"Dendrite/pkg/telemetry/exporter"
)

func handleTelemetryCommand(ctx context.Context, args []string) {
	daemon := telemetry.NewDaemon(telemetry.DaemonConfig{
		Interval: 1 * time.Second,
	}, nil, nil, nil, nil)

	if len(args) > 0 && (args[0] == "metrics" || args[0] == "export" || args[0] == "prometheus") {
		exp := exporter.NewPrometheusExporter(daemon)
		if err := exp.WriteMetrics(ctx, os.Stdout); err != nil {
			fmt.Printf("[-] Failed to write Prometheus metrics: %v\n", err)
		}
		return
	}

	rep := daemon.CollectOnce(ctx)
	fmt.Printf("[+] Instantaneous Telemetry Snapshot:\n")
	fmt.Printf("    - Timestamp:        %s\n", rep.Timestamp.Format(time.RFC3339))
	fmt.Printf("    - SoC Temperature:  %.1f°C (Alert: %v, Critical: %v)\n", rep.SoCTempC, rep.ThermalAlert, rep.ThermalCritical)
	fmt.Printf("    - 24h Spend:        $%.2f / $%.2f daily budget\n", rep.Spend24hUSD, rep.DailyBudgetUSD)
	fmt.Printf("    - 7d Spend:         $%.2f\n", rep.Spend7dUSD)
	fmt.Println("    - Node Health:")
	for node, status := range rep.NodeStatus {
		fmt.Printf("        * %-10s -> %s\n", node, status)
	}
}
