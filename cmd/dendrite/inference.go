package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"Dendrite/pkg/devtool"
	"Dendrite/pkg/gateway"
	"Dendrite/pkg/inference"
	"Dendrite/pkg/statedir"

	"github.com/spf13/cobra"
)

// newSpikeCmd builds `dendrite spike` (alias inference): the multi-provider inference engine manager.
func newSpikeCmd() *cobra.Command {
	cmd := newGroupCmd("spike", "Unified multi-provider inference engine gateway (Ollama, OxiLLaMa, vLLM, TGI)", "cleft",
		"inference", "action-potential", "inf")
	cmd.Example = "dendrite spike list\n" +
		"dendrite spike probe ollama-metal\n" +
		"dendrite spike probe --endpoint http://10.0.0.5:11434\n" +
		"dendrite spike migrate qwen2.5-coder:7b --to oxillama --delete-source\n" +
		"dendrite spike zero-cost add my-model --env"
	cmd.AddCommand(
		newSpikeListCmd(),
		newSpikeProbeCmd(),
		newSpikeSwitchCmd(),
		newSpikeRenderCmd(),
		newSpikeMigrateCmd(),
		newSpikeServeCmd(),
		newSpikeStopCmd(),
		newSpikeZeroCostCmd(),
	)
	return cmd
}

func newSpikeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered engines, their models and the failover cascade",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpikeList(inference.DefaultManager(), cmd.OutOrStdout())
		},
	}
}

func runSpikeList(mgr *inference.Manager, out io.Writer) error {
	engines := mgr.ListEngines()
	cascade := mgr.GetCascadeOrder()
	if globals.JSON {
		type engineInfo struct {
			ID       string   `json:"id"`
			Type     string   `json:"type"`
			Endpoint string   `json:"endpoint"`
			Models   []string `json:"models"`
			ZeroCost bool     `json:"zero_cost"`
		}
		list := make([]engineInfo, 0, len(engines))
		for _, eng := range engines {
			cfg := eng.Config()
			list = append(list, engineInfo{eng.ID(), string(eng.Type()), cfg.Endpoint, cfg.Models, inference.IsEngineZeroCost(eng)})
		}
		return writeJSON(out, map[string]any{"engines": list, "cascade": cascade})
	}
	fmt.Fprintf(out, "[+] Registered Multi-Provider Inference Engines (%d):\n", len(engines))
	for _, eng := range engines {
		cfg := eng.Config()
		zeroCostTag := ""
		if inference.IsEngineZeroCost(eng) {
			zeroCostTag = " [Zero-Cost / Self-Hosted]"
		}
		modelsStr := strings.Join(cfg.Models, ", ")
		if modelsStr == "" {
			modelsStr = "(auto-discover)"
		}
		fmt.Fprintf(out, "    [%s] %s (%s)%s\n", eng.Type(), eng.ID(), cfg.Endpoint, zeroCostTag)
		fmt.Fprintf(out, "        Models: %s\n", modelsStr)
	}
	fmt.Fprintln(out, "\n[+] Dynamic Cascade Failover Priority Order:")
	for i, id := range cascade {
		fmt.Fprintf(out, "    %d. %s\n", i+1, id)
	}
	return nil
}

func newSpikeProbeCmd() *cobra.Command {
	var endpoint string
	cmd := &cobra.Command{
		Use:   "probe [engine-id]",
		Short: "Check engine reachability, latency and models (all engines, one engine, or a raw Ollama endpoint)",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if endpoint != "" {
				if len(args) > 0 {
					return usageErrorf("give either an engine id or --endpoint, not both")
				}
				return runSpikeProbeEndpoint(cmd.Context(), endpoint, out)
			}
			id := ""
			if len(args) > 0 {
				id = args[0]
			}
			return runSpikeProbe(cmd.Context(), inference.DefaultManager(), id, out)
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Probe an Ollama-compatible endpoint URL instead of a registered engine")
	return cmd
}

func runSpikeProbe(ctx context.Context, mgr *inference.Manager, id string, out io.Writer) error {
	var results []inference.EngineHealth
	if id != "" {
		eng, err := mgr.GetEngine(id)
		if err != nil {
			return usageErrorf("unknown inference engine %q", id)
		}
		if !globals.JSON {
			fmt.Fprintf(out, "[+] Probing engine '%s' (%s at %s)...\n", eng.ID(), eng.Type(), eng.Config().Endpoint)
		}
		results = []inference.EngineHealth{eng.Health(ctx)}
	} else {
		if !globals.JSON {
			fmt.Fprintln(out, "[+] Probing all registered inference engines (< 2s concurrent health check)...")
		}
		all := mgr.PollHealthAll(ctx)
		for _, id := range slices.Sorted(maps.Keys(all)) {
			results = append(results, all[id])
		}
	}
	if globals.JSON {
		return writeJSON(out, results)
	}
	for _, h := range results {
		printEngineHealth(out, h)
	}
	return nil
}

func runSpikeProbeEndpoint(ctx context.Context, endpoint string, out io.Writer) error {
	fmt.Fprintf(out, "[+] Probing Ollama at %s...\n", endpoint)
	status, models, err := devtool.ProbeOllama(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("probe %s: %w", endpoint, err)
	}
	fmt.Fprintf(out, "[✓] Status: Reachable (Latency: %v)\n", status.Latency)
	fmt.Fprintf(out, "[✓] Models (%d):\n", len(models))
	for _, m := range models {
		fmt.Fprintf(out, "    - %s\n", m.Name)
	}
	return nil
}

func newSpikeSwitchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <engine-id>",
		Short: "Move an engine to the front of the failover cascade",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpikeSwitch(inference.DefaultManager(), args[0], cmd.OutOrStdout())
		},
	}
}

func runSpikeSwitch(mgr *inference.Manager, primaryID string, out io.Writer) error {
	if _, err := mgr.GetEngine(primaryID); err != nil {
		return usageErrorf("unknown inference engine %q", primaryID)
	}
	newOrder := []string{primaryID}
	for _, id := range mgr.GetCascadeOrder() {
		if id != primaryID {
			newOrder = append(newOrder, id)
		}
	}
	if err := mgr.SetCascadeOrder(newOrder); err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] Successfully updated cascade order. Primary inference engine is now: %s\n", primaryID)
	fmt.Fprintf(out, "    New cascade: %s\n", strings.Join(newOrder, " -> "))
	return nil
}

func newSpikeRenderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "render",
		Short: "Render the registered engines' model routes as LiteLLM config",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			yamlBytes, err := gateway.RenderForInferenceManager(inference.DefaultManager())
			if err != nil {
				return fmt.Errorf("render LiteLLM configuration: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[+] Rendered LiteLLM Configuration for Multi-Engine Inference:\n\n%s\n", string(yamlBytes))
			return nil
		},
	}
}

func newSpikeMigrateCmd() *cobra.Command {
	opts := inference.ModelMigrationOptions{SourceEngine: "ollama", TargetEngine: "oxillama"}
	cmd := &cobra.Command{
		Use:     "migrate <model>",
		Aliases: []string{"move"},
		Short:   "Copy a model's blobs between engines (default Ollama to OxiLLaMa), optionally evicting the source",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ModelName = args[0]
			return runSpikeMigrate(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.SourceEngine, "from", opts.SourceEngine, "Source engine")
	f.StringVar(&opts.TargetEngine, "to", opts.TargetEngine, "Target engine")
	f.BoolVar(&opts.DeleteSource, "delete-source", false, "Evict the model from the source after copying")
	addDryRunFlag(cmd, &opts.DryRun)
	return cmd
}

func runSpikeMigrate(ctx context.Context, opts inference.ModelMigrationOptions, out io.Writer) error {
	fmt.Fprintf(out, "[+] Initiating model migration for '%s' (%s -> %s)...\n", opts.ModelName, opts.SourceEngine, opts.TargetEngine)
	res, err := inference.NewModelMigrator("", "", nil).Migrate(ctx, opts)
	if err != nil {
		return fmt.Errorf("migration: %w", err)
	}
	fmt.Fprintf(out, "[✓] Successfully migrated model '%s' to %s in %v\n", res.ModelName, res.TargetEngine, res.Duration)
	fmt.Fprintf(out, "    * Blobs Migrated:     %d\n", res.BlobsMigrated)
	fmt.Fprintf(out, "    * Total Data Size:    %.2f GB (%d bytes)\n", float64(res.TotalBytesMigrated)/(1024*1024*1024), res.TotalBytesMigrated)
	fmt.Fprintf(out, "    * Target Manifest:    %s\n", res.TargetManifest)
	switch {
	case res.SourceEvicted:
		fmt.Fprintf(out, "    * Source Evicted:     YES (Reclaimed %.2f GB memory/disk from %s)\n", float64(res.BytesFreed)/(1024*1024*1024), res.SourceEngine)
	case opts.DeleteSource:
		fmt.Fprintf(out, "    * Source Evicted:     NO (%s)\n", res.Error)
	default:
		fmt.Fprintln(out, "    * Source Retained:    (pass --delete-source to evict from source and reclaim memory)")
	}
	return nil
}

func addPortFlag(cmd *cobra.Command, p *int) {
	cmd.Flags().IntVar(p, "port", 8080, "OxiLLaMa server port")
}

func newSpikeServeCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:     "serve",
		Aliases: []string{"start"},
		Short:   "Run the OxiLLaMa OpenAI-compatible server in the foreground (Ctrl+C stops it)",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runSpikeServe(ctx, port, cmd.OutOrStdout())
		},
	}
	addPortFlag(cmd, &port)
	return cmd
}

func runSpikeServe(ctx context.Context, port int, out io.Writer) error {
	srv := inference.NewOxiLLaMaServer(inference.OxiLLaMaServerConfig{Host: "127.0.0.1", Port: port})
	if err := srv.Start(); err != nil {
		return fmt.Errorf("start OxiLLaMa server: %w", err)
	}
	defer func() { _ = srv.Stop(context.Background()) }()
	fmt.Fprintf(out, "[✓] OxiLLaMa Pure-Rust OpenAI Server listening at http://127.0.0.1:%d\n", port)
	fmt.Fprintf(out, "    * Models Endpoint:     http://127.0.0.1:%d/v1/models\n", port)
	fmt.Fprintf(out, "    * Chat Endpoint:       http://127.0.0.1:%d/v1/chat/completions\n", port)
	fmt.Fprintln(out, "    * Storage Directory:   ~/.oxillama/models")
	fmt.Fprintln(out, "[+] Press Ctrl+C to stop.")
	<-ctx.Done()
	return nil
}

func newSpikeStopCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:     "stop",
		Aliases: []string{"kill"},
		Short:   "Stop a running OxiLLaMa server and free its port",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpikeStop(cmd.Context(), port, cmd.OutOrStdout())
		},
	}
	addPortFlag(cmd, &port)
	return cmd
}

func runSpikeStop(ctx context.Context, port int, out io.Writer) error {
	fmt.Fprintf(out, "[+] Stopping OxiLLaMa server on port %d...\n", port)
	pidsOut, _ := exec.CommandContext(ctx, "lsof", "-ti", fmt.Sprintf(":%d", port)).Output()
	stopped := 0
	for _, pidStr := range strings.Fields(string(pidsOut)) {
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			if proc, err := os.FindProcess(pid); err == nil && proc.Kill() == nil {
				stopped++
			}
		}
	}
	_ = exec.CommandContext(ctx, "pkill", "-f", "oxillama serve").Run()
	if stopped > 0 {
		fmt.Fprintf(out, "[✓] Successfully stopped OxiLLaMa server (terminated %d process(es) on port %d)\n", stopped, port)
	} else {
		fmt.Fprintf(out, "[✓] OxiLLaMa server is not currently running on port %d.\n", port)
	}
	return nil
}

func newSpikeZeroCostCmd() *cobra.Command {
	cmd := newGroupCmd("zero-cost", "Manage the zero-cost (self-hosted) model and prefix rules", "", "zerocost")
	cmd.Example = "dendrite spike zero-cost list\n" +
		"dendrite spike zero-cost add local/ --prefix\n" +
		"dendrite spike zero-cost test qwen2.5-coder:14b"

	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List zero-cost models and prefixes",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runZeroCostList(inference.GetZeroCostRegistry(), cmd.OutOrStdout())
		},
	}

	var forcePrefix, addEnv bool
	add := &cobra.Command{
		Use:   "add <model|prefix/>",
		Short: "Register a zero-cost model or prefix rule",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runZeroCostAdd(inference.GetZeroCostRegistry(), args[0], forcePrefix, addEnv, cmd.OutOrStdout())
		},
	}
	add.Flags().BoolVarP(&forcePrefix, "prefix", "p", false, "Treat the pattern as a prefix rule")
	add.Flags().BoolVarP(&addEnv, "env", "e", false, "Also write the rules to .env (always done when .env exists)")

	var removeEnv bool
	remove := &cobra.Command{
		Use:     "remove <model|prefix/>",
		Aliases: []string{"rm", "delete"},
		Short:   "Unregister a zero-cost model or prefix rule",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runZeroCostRemove(inference.GetZeroCostRegistry(), args[0], removeEnv, cmd.OutOrStdout())
		},
	}
	remove.Flags().BoolVarP(&removeEnv, "env", "e", false, "Also write the rules to .env (always done when .env exists)")

	test := &cobra.Command{
		Use:     "test <model>",
		Aliases: []string{"check"},
		Short:   "Check whether a model qualifies as zero-cost",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ok, reason := inference.GetZeroCostRegistry().Test(args[0])
			if globals.JSON {
				return writeJSON(cmd.OutOrStdout(), map[string]any{"model": args[0], "zero_cost": ok, "reason": reason})
			}
			mark := "[-]"
			if ok {
				mark = "[✓]"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s Zero-Cost: %s\n    Reason: %s\n", mark, strings.ToUpper(strconv.FormatBool(ok)), reason)
			return nil
		},
	}

	reset := &cobra.Command{
		Use:   "reset",
		Short: "Restore the default rules plus the active .env configuration",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			reg := inference.GetZeroCostRegistry()
			reg.Reset()
			if err := os.Remove(statedir.Resolve(inference.DefaultZeroCostConfigFile)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove zero-cost config: %w", err)
			}
			reg.AutoLoad()
			fmt.Fprintln(cmd.OutOrStdout(), "[✓] Successfully reset zero-cost models to defaults + active .env configuration.")
			return nil
		},
	}

	cmd.AddCommand(list, add, remove, test, reset)
	return cmd
}

func runZeroCostList(reg *inference.ZeroCostRegistry, out io.Writer) error {
	details := reg.GetDetails()
	if globals.JSON {
		return writeJSON(out, details)
	}
	rule := "    --------------------------------------------------------------------------------"
	fmt.Fprintf(out, "[+] Configured Zero-Cost & Self-Hosted Models / Prefixes (%d active):\n", details.Total)
	fmt.Fprintln(out, rule)
	fmt.Fprintf(out, "    %-32s %-10s %-12s %s\n", "PATTERN", "KIND", "SOURCE", "STATUS")
	fmt.Fprintln(out, rule)
	status := func(active bool) string {
		if active {
			return "Active"
		}
		return "Overridden / Disabled"
	}
	for _, m := range details.Models {
		fmt.Fprintf(out, "    %-32s %-10s %-12s %s\n", m.Pattern, "Exact", m.Source, status(m.Active))
	}
	for _, p := range details.Prefixes {
		fmt.Fprintf(out, "    %-32s %-10s %-12s %s\n", p.Pattern, "Prefix", p.Source, status(p.Active))
	}
	fmt.Fprintln(out, rule)
	return nil
}

// saveZeroCost persists the registry to the state dir, and to .env when it exists or syncEnv is set.
func saveZeroCost(reg *inference.ZeroCostRegistry, syncEnv bool, out io.Writer) error {
	path := statedir.Resolve(inference.DefaultZeroCostConfigFile)
	if err := reg.SaveToFile(path); err != nil {
		return fmt.Errorf("save zero-cost config: %w", err)
	}
	fmt.Fprintf(out, "    Saved to dynamic config: %s\n", path)
	envPath := ".env"
	if _, err := os.Stat(envPath); err == nil || syncEnv {
		if err := reg.SaveToEnvFile(envPath); err != nil {
			return fmt.Errorf("write %s: %w", envPath, err)
		}
		fmt.Fprintf(out, "    Synchronized with environment file: %s\n", envPath)
	}
	return nil
}

func zeroCostKind(isPrefix bool) string {
	if isPrefix {
		return "prefix rule"
	}
	return "exact model"
}

func runZeroCostAdd(reg *inference.ZeroCostRegistry, target string, forcePrefix, syncEnv bool, out io.Writer) error {
	var ok, isPrefix bool
	if forcePrefix {
		ok, isPrefix = reg.AddPrefix(target), true
	} else {
		ok, isPrefix = reg.Add(target)
	}
	if !ok {
		return usageErrorf("invalid zero-cost pattern %q", target)
	}
	fmt.Fprintf(out, "[✓] Registered '%s' as zero-cost %s\n", target, zeroCostKind(isPrefix))
	return saveZeroCost(reg, syncEnv, out)
}

// runZeroCostRemove disables a pattern; removing a built-in default records an override.
func runZeroCostRemove(reg *inference.ZeroCostRegistry, target string, syncEnv bool, out io.Writer) error {
	ok, isPrefix := reg.Remove(target)
	if !ok {
		return usageErrorf("invalid zero-cost pattern %q", target)
	}
	fmt.Fprintf(out, "[✓] Removed '%s' from zero-cost %s\n", target, zeroCostKind(isPrefix))
	return saveZeroCost(reg, syncEnv, out)
}

func printEngineHealth(out io.Writer, h inference.EngineHealth) {
	status := "Offline"
	if h.Reachable {
		status = "Online"
	}
	costInfo := "commercial"
	if h.ZeroCost {
		costInfo = "zero-cost"
	}
	fmt.Fprintf(out, "    * [%s] Engine: %-15s Type: %-18s Latency: %-8v Cost: %s\n",
		status, h.EngineID, h.Type, h.Latency, costInfo)
	if len(h.Models) > 0 {
		fmt.Fprintf(out, "        Models: %s\n", strings.Join(h.Models, ", "))
	}
	if h.Error != "" {
		fmt.Fprintf(out, "        Error:  %s\n", h.Error)
	}
}
