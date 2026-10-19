package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"Dendrite/pkg/topology"
	"Dendrite/pkg/topology/advisor"

	"github.com/spf13/cobra"
)

// newAxonCmd builds `dendrite axon` (alias topology): multi-node placement and topology inspection.
func newAxonCmd() *cobra.Command {
	cmd := newGroupCmd("axon", "Multi-node placement models and hardware service router", "axon",
		"topology", "axon-pathway", "top")
	cmd.Example = "dendrite axon advisor --nodes local=localhost,gpu=10.0.0.5\n" +
		"dendrite axon advisor --json\n" +
		"dendrite axon inspect distributed"

	var nodes string
	advisorCmd := &cobra.Command{
		Use:     "advisor",
		Aliases: []string{"advise"},
		Short:   "Probe compute nodes and recommend a service placement",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAxonAdvisor(cmd.Context(), parseNodeTargets(nodes), cmd.OutOrStdout())
		},
	}
	advisorCmd.Flags().StringVar(&nodes, "nodes", "local=localhost", "Comma-separated nodes as id=host (or just host)")

	inspect := &cobra.Command{
		Use:       "inspect [scenario]",
		Aliases:   []string{"show"},
		Short:     "Show stack placements and compose profiles for a scenario (default local-hybrid)",
		Args:      maxArgs(1),
		ValidArgs: scenarioNames(),
		RunE: func(cmd *cobra.Command, args []string) error {
			sc, err := parseScenarioArg(args)
			if err != nil {
				return err
			}
			return runAxonInspect(sc, cmd.OutOrStdout())
		},
	}

	cmd.AddCommand(advisorCmd, inspect)
	return cmd
}

func runAxonInspect(sc topology.Scenario, out io.Writer) error {
	cfg, err := topology.NewTopologyConfig(sc, tailscaleEndpoints())
	if err != nil {
		return fmt.Errorf("construct topology: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, cfg)
	}
	fmt.Fprintf(out, "[+] Active Scenario: %s\n", cfg.Scenario)
	fmt.Fprintln(out, "[+] Stack Placements:")
	comps := make([]string, 0, len(cfg.StackPlacement))
	for comp := range cfg.StackPlacement {
		comps = append(comps, string(comp))
	}
	sort.Strings(comps)
	for _, comp := range comps {
		fmt.Fprintf(out, "    - %-15s -> %s\n", comp, cfg.StackPlacement[topology.StackComponent(comp)])
	}
	fmt.Fprintln(out, "[+] Node Compose Profiles:")
	for _, node := range topology.AllComputeNodes() {
		if profiles := cfg.ActiveComposeProfiles(node); len(profiles) > 0 {
			fmt.Fprintf(out, "    - %-10s: %s\n", node, strings.Join(profiles, ", "))
		}
	}
	return nil
}

func runAxonAdvisor(ctx context.Context, targets []advisor.NodeTarget, out io.Writer) error {
	// In JSON mode stdout carries only the plan.
	progress := out
	if globals.JSON {
		progress = io.Discard
	}
	fmt.Fprintf(progress, "[+] Probing %d compute node(s) for hardware metrics...\n", len(targets))
	hardwareList, err := advisor.NewHardwareProbe(nil).ProbeAll(ctx, targets)
	if err != nil {
		return fmt.Errorf("probe nodes: %w", err)
	}

	fmt.Fprintf(progress, "[✓] Discovered %d compute node(s):\n", len(hardwareList))
	for _, hw := range hardwareList {
		gpuInfo := "None"
		if hw.HasGPU {
			gpuInfo = fmt.Sprintf("%s (%d MB VRAM)", hw.GPUType, hw.VRAMMB)
		}
		fmt.Fprintf(progress, "    - %-12s: %s/%s, %d CPU cores, %d MB RAM (%d MB free), %d GB disk, GPU: %s\n",
			hw.NodeID, hw.OS, hw.Arch, hw.CPUCores, hw.TotalRAMMB, hw.FreeRAMMB, hw.DiskFreeGB, gpuInfo)
	}

	plan, err := advisor.NewPlacementAdvisor().Advise(hardwareList, advisor.DefaultDendriteServices())
	if err != nil {
		return fmt.Errorf("placement advisor: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, plan)
	}
	fmt.Fprint(out, plan.FormatTable())
	return nil
}

func parseNodeTargets(nodeArgs string) []advisor.NodeTarget {
	var targets []advisor.NodeTarget
	for _, tok := range strings.Split(nodeArgs, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		id, host := tok, tok
		if before, after, ok := strings.Cut(tok, "="); ok {
			id, host = strings.TrimSpace(before), strings.TrimSpace(after)
		}
		isLocal := host == "localhost" || host == "127.0.0.1" || host == "local"
		targets = append(targets, advisor.NodeTarget{ID: id, Host: host, IsLocal: isLocal})
	}
	if len(targets) == 0 {
		targets = append(targets, advisor.NodeTarget{ID: "local", Host: "localhost", IsLocal: true})
	}
	return targets
}
