package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/node/migrate"

	"github.com/spf13/cobra"
)

// newTranslocateCmd builds `dendrite translocate` (alias move): capacity planning and SSH service migration.
func newTranslocateCmd() *cobra.Command {
	cmd := newGroupCmd("translocate", "Zero-credential interactive service migration across SSH nodes", "axon",
		"move", "axonal-translocation", "mv")
	cmd.Example = "dendrite translocate plan --ram 4GB --disk 1TB\n" +
		"dendrite translocate run plane litellm --ssh user@debian-server --dry-run\n" +
		"dendrite translocate run debian --ssh user@debian-server --yes"

	var ramStr, diskStr string
	plan := &cobra.Command{
		Use:   "plan",
		Short: "Show which services fit on a target server of the given size",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ram, disk, err := parseCapacity(ramStr, diskStr)
			if err != nil {
				return err
			}
			p := migrate.EvaluateDebianServer(ram, disk)
			if globals.JSON {
				return writeJSON(cmd.OutOrStdout(), p)
			}
			printDebianCapacityPlan(cmd.OutOrStdout(), p)
			return nil
		},
	}
	addCapacityFlags(plan, &ramStr, &diskStr)
	cmd.AddCommand(plan, newTranslocateRunCmd())
	return cmd
}

func addCapacityFlags(cmd *cobra.Command, ram, disk *string) {
	cmd.Flags().StringVar(ram, "ram", "4GB", "Target RAM (e.g. 4GB, 4096MB)")
	cmd.Flags().StringVar(disk, "disk", "1TB", "Target disk (e.g. 1TB, 1000GB)")
}

// parseCapacity reads --ram and --disk; an unreadable size is a usage error.
func parseCapacity(ramStr, diskStr string) (ramMB, diskGB uint64, err error) {
	ramMB, ok := parseSizeToMB(ramStr)
	if !ok {
		return 0, 0, usageErrorf("invalid --ram %q: use a size like 4GB or 4096MB", ramStr)
	}
	diskGB, ok = parseSizeToGB(diskStr)
	if !ok {
		return 0, 0, usageErrorf("invalid --disk %q: use a size like 1TB or 1000GB", diskStr)
	}
	return ramMB, diskGB, nil
}

func newTranslocateRunCmd() *cobra.Command {
	req := migrate.MigrationRequest{SSHPort: 22, Timeout: 120 * time.Second}
	var ramStr, diskStr string
	var noPrune bool
	cmd := &cobra.Command{
		Use:   "run <service>...",
		Short: "Move services (or 'debian' for the full eligible set) to a host over SSH",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ram, disk, err := parseCapacity(ramStr, diskStr)
			if err != nil {
				return err
			}
			for _, a := range args {
				for _, s := range strings.Split(a, ",") {
					if s = strings.TrimSpace(s); s != "" {
						req.Services = append(req.Services, s)
					}
				}
			}
			req.TargetRAMMB, req.TargetDiskGB = ram, disk
			req.PruneLocal = !noPrune
			req.AutoApprove = globals.Yes
			req.In, req.Out = cmd.InOrStdin(), cmd.OutOrStdout()
			return runTranslocate(cmd.Context(), req, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVarP(&req.TargetHost, "ssh", "s", "", "Target SSH host (user@host or host)")
	f.IntVarP(&req.SSHPort, "port", "p", req.SSHPort, "SSH port")
	f.StringVarP(&req.SSHUser, "user", "u", "", "SSH user (default from the host or the deployer)")
	f.StringVarP(&req.KeyPath, "key", "i", "", "SSH identity file (default: ssh-agent)")
	addCapacityFlags(cmd, &ramStr, &diskStr)
	addDryRunFlag(cmd, &req.DryRun)
	f.BoolVar(&noPrune, "no-prune", false, "Keep the local containers running after the move")
	f.DurationVar(&req.Timeout, "timeout", req.Timeout, "Remote health check timeout")
	requireFlags(cmd, "ssh")
	return cmd
}

func runTranslocate(ctx context.Context, req migrate.MigrationRequest, out io.Writer) error {
	fmt.Fprintln(out, "[+] Initializing Dendrite Service Migration Engine...")
	fmt.Fprintf(out, "    * Target Host:   %s (Port: %d)\n", req.TargetHost, req.SSHPort)
	fmt.Fprintf(out, "    * Services:      %s\n", strings.Join(req.Services, ", "))
	fmt.Fprintf(out, "    * Target Node:   RAM: %d MB, Disk: %d GB\n", req.TargetRAMMB, req.TargetDiskGB)
	fmt.Fprintf(out, "    * Dry Run Mode:  %v\n", req.DryRun)
	fmt.Fprintf(out, "    * Prune Local:   %v\n", req.PruneLocal)

	res, err := migrate.NewMigrator(nil, nil).Migrate(ctx, req)
	if err != nil {
		if res != nil {
			for _, step := range res.Steps {
				fmt.Fprintf(out, "    [%s] %s: %s\n", step.Status, step.Phase, step.Details)
			}
		}
		return fmt.Errorf("service migration: %w", err)
	}
	fmt.Fprintf(out, "\n[✓] Service Migration Completed Successfully in %v!\n", res.TotalDuration)
	for _, step := range res.Steps {
		fmt.Fprintf(out, "    [%s] %s: %s (%v)\n", step.Status, step.Phase, step.Details, step.Duration)
	}
	if len(res.UpdatedEndpoints) > 0 {
		fmt.Fprintln(out, "\n[+] Updated Cluster Service Endpoints:")
		for _, svc := range slices.Sorted(maps.Keys(res.UpdatedEndpoints)) {
			fmt.Fprintf(out, "    * %-15s -> %s\n", svc, res.UpdatedEndpoints[svc])
		}
	}
	return nil
}

func printDebianCapacityPlan(out io.Writer, plan migrate.DebianCapacityPlan) {
	fmt.Fprintln(out, "==========================================================================================")
	fmt.Fprintf(out, "              DENDRITE SERVICE CAPACITY EVALUATION: %s\n", strings.ToUpper(plan.ServerProfile))
	fmt.Fprintln(out, "==========================================================================================")
	fmt.Fprintf(out, "  * Total Server RAM : %d MB (%.1f GB)\n", plan.TotalRAMMB, float64(plan.TotalRAMMB)/1024.0)
	fmt.Fprintf(out, "  * Total Server Disk: %d GB (%.1f TB)\n", plan.TotalDiskGB, float64(plan.TotalDiskGB)/1000.0)
	fmt.Fprintf(out, "  * OS System Reserve: 1024 MB (Debian Linux kernel, systemd, SSH daemon)\n")
	fmt.Fprintf(out, "  * Net Service RAM  : %d MB (Allocated: %d MB, Remaining: %d MB)\n",
		plan.TotalRAMMB-1024, plan.AllocatedRAMMB, plan.AvailableRAMMB)
	fmt.Fprintf(out, "  * Net Service Disk : %d GB (Allocated: %d GB, Remaining: %d GB)\n",
		plan.TotalDiskGB, plan.AllocatedDiskGB, plan.AvailableDiskGB)
	fmt.Fprintln(out, "------------------------------------------------------------------------------------------")

	fmt.Fprintln(out, "[+] SERVICES ELIGIBLE TO LIVE ON DEBIAN SERVER (CUMULATIVE FIT):")
	for idx, e := range plan.EligibleServices {
		fmt.Fprintf(out, "  %d. %-15s [%s]\n", idx+1, e.Service, e.DisplayName)
		fmt.Fprintf(out, "     Footprint: %d MB RAM, %d GB Disk\n", e.RAMMB, e.DiskGB)
		fmt.Fprintf(out, "     Status   : [✓] %s\n", e.Reason)
	}

	if len(plan.ExcludedServices) > 0 {
		fmt.Fprintln(out, "\n[-] SERVICES EXCLUDED / RETAINED ON LOCAL WORKSTATION:")
		for idx, e := range plan.ExcludedServices {
			fmt.Fprintf(out, "  %d. %-15s [%s]\n", idx+1, e.Service, e.DisplayName)
			fmt.Fprintf(out, "     Footprint: %d MB RAM, %d GB Disk\n", e.RAMMB, e.DiskGB)
			fmt.Fprintf(out, "     Constraint: [!] %s\n", e.Reason)
		}
	}

	fmt.Fprintln(out, "------------------------------------------------------------------------------------------")
	fmt.Fprintf(out, "[+] RECOMMENDED EXECUTION COMMAND:\n")
	fmt.Fprintf(out, "    %s\n", plan.RecommendedAction)
	fmt.Fprintln(out, "==========================================================================================")
}

// parseSizeToMB reads sizes like 4GB, 4096MB or a bare number of megabytes.
func parseSizeToMB(raw string) (uint64, bool) {
	return parseSize(raw, map[string]uint64{"GB": 1024, "MB": 1})
}

// parseSizeToGB reads sizes like 1TB, 1000GB or a bare number of gigabytes.
func parseSizeToGB(raw string) (uint64, bool) {
	return parseSize(raw, map[string]uint64{"TB": 1000, "GB": 1})
}

func parseSize(raw string, units map[string]uint64) (uint64, bool) {
	clean := strings.ToUpper(strings.TrimSpace(raw))
	mult := uint64(1)
	for unit, m := range units {
		if strings.HasSuffix(clean, unit) {
			clean, mult = strings.TrimSuffix(clean, unit), m
			break
		}
	}
	n, err := strconv.ParseUint(clean, 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	return n * mult, true
}
