package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"Dendrite/pkg/node/migrate"
)

func printMoveUsage() {
	fmt.Println("Dendrite Service Migration Engine (Zero-Credential SSH)")
	fmt.Println("\nUsage:")
	fmt.Println("  dendrite move <service[,service...]> to --ssh <target> [flags]")
	fmt.Println("  dendrite move debian --ssh <target> [flags]")
	fmt.Println("  dendrite move plan [--ram <size>] [--disk <size>]")
	fmt.Println("\nExamples:")
	fmt.Println("  dendrite move plane to --ssh user@debian-server")
	fmt.Println("  dendrite move plane,litellm,database to --ssh user@debian-server --dry-run")
	fmt.Println("  dendrite move debian --ssh user@debian-server --yes")
	fmt.Println("  dendrite move plan --ram 4GB --disk 1TB")
	fmt.Println("\nFlags:")
	fmt.Println("  --ssh, -s <host>       Target SSH host (e.g. user@debian-server or debian-server)")
	fmt.Println("  --port, -p <port>      SSH port (default 22)")
	fmt.Println("  --user, -u <user>      SSH user (default from target host or deployer)")
	fmt.Println("  --key, -i <path>       Optional SSH identity key path (uses ssh-agent by default)")
	fmt.Println("  --ram <size>           Target host RAM capacity (e.g. 4GB, 4096MB) [default 4GB]")
	fmt.Println("  --disk <size>          Target host Disk capacity (e.g. 1TB, 1000GB) [default 1TB]")
	fmt.Println("  --dry-run              Simulate migration without modifying local or remote containers")
	fmt.Println("  --yes, -y              Auto-approve migration without interactive terminal prompt")
	fmt.Println("  --no-prune             Keep local containers running after successful remote deployment")
	fmt.Println("  --timeout <seconds>    Remote health check timeout (default 120s)")
}

func handleMoveCommand(ctx context.Context, args []string) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printMoveUsage()
		return
	}

	// Subcommand: plan
	if args[0] == "plan" {
		ramMB := uint64(4096)
		diskGB := uint64(1000)
		for i := 1; i < len(args); i++ {
			if args[i] == "--ram" && i+1 < len(args) {
				ramMB = parseSizeToMB(args[i+1], 4096)
				i++
			} else if args[i] == "--disk" && i+1 < len(args) {
				diskGB = parseSizeToGB(args[i+1], 1000)
				i++
			}
		}

		plan := migrate.EvaluateDebianServer(ramMB, diskGB)
		printDebianCapacityPlan(plan)
		return
	}

	// Parsing migration targets and flags
	var rawServices []string
	var targetHost string
	sshPort := 22
	var sshUser string
	var keyPath string
	targetRAMMB := uint64(4096)
	targetDiskGB := uint64(1000)
	dryRun := false
	autoApprove := false
	pruneLocal := true
	timeout := 120 * time.Second

	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "to":
			// skip noise token "to" (e.g. "dendrite move plane to --ssh target")
			i++
		case arg == "--ssh" || arg == "-s":
			if i+1 < len(args) {
				targetHost = args[i+1]
				i += 2
			} else {
				i++
			}
		case arg == "--port" || arg == "-p":
			if i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil {
					sshPort = p
				}
				i += 2
			} else {
				i++
			}
		case arg == "--user" || arg == "-u":
			if i+1 < len(args) {
				sshUser = args[i+1]
				i += 2
			} else {
				i++
			}
		case arg == "--key" || arg == "-i":
			if i+1 < len(args) {
				keyPath = args[i+1]
				i += 2
			} else {
				i++
			}
		case arg == "--ram":
			if i+1 < len(args) {
				targetRAMMB = parseSizeToMB(args[i+1], 4096)
				i += 2
			} else {
				i++
			}
		case arg == "--disk":
			if i+1 < len(args) {
				targetDiskGB = parseSizeToGB(args[i+1], 1000)
				i += 2
			} else {
				i++
			}
		case arg == "--dry-run":
			dryRun = true
			i++
		case arg == "--yes" || arg == "-y":
			autoApprove = true
			i++
		case arg == "--no-prune":
			pruneLocal = false
			i++
		case arg == "--timeout":
			if i+1 < len(args) {
				if sec, err := strconv.Atoi(args[i+1]); err == nil {
					timeout = time.Duration(sec) * time.Second
				}
				i += 2
			} else {
				i++
			}
		default:
			if !strings.HasPrefix(arg, "-") {
				// comma-separated service list
				parts := strings.Split(arg, ",")
				for _, p := range parts {
					trimmed := strings.TrimSpace(p)
					if trimmed != "" {
						rawServices = append(rawServices, trimmed)
					}
				}
			}
			i++
		}
	}

	if len(rawServices) == 0 {
		fmt.Println("[-] Please specify service(s) to move (e.g. plane, litellm, database, or 'debian').")
		fmt.Println("    Run 'dendrite move --help' for details.")
		return
	}

	if targetHost == "" {
		fmt.Println("[-] Target SSH host is required. Specify with '--ssh user@hostname'.")
		return
	}

	migrator := migrate.NewMigrator(nil, nil)
	req := migrate.MigrationRequest{
		Services:     rawServices,
		TargetHost:   targetHost,
		SSHPort:      sshPort,
		SSHUser:      sshUser,
		KeyPath:      keyPath,
		TargetRAMMB:  targetRAMMB,
		TargetDiskGB: targetDiskGB,
		AutoApprove:  autoApprove,
		DryRun:       dryRun,
		PruneLocal:   pruneLocal,
		Timeout:      timeout,
		In:           os.Stdin,
		Out:          os.Stdout,
	}

	fmt.Printf("[+] Initializing Dendrite Service Migration Engine...\n")
	fmt.Printf("    * Target Host:   %s (Port: %d)\n", targetHost, sshPort)
	fmt.Printf("    * Services:      %s\n", strings.Join(rawServices, ", "))
	fmt.Printf("    * Target Node:   RAM: %d MB, Disk: %d GB\n", targetRAMMB, targetDiskGB)
	fmt.Printf("    * Dry Run Mode:  %v\n", dryRun)
	fmt.Printf("    * Prune Local:   %v\n", pruneLocal)

	res, err := migrator.Migrate(ctx, req)
	if err != nil {
		fmt.Printf("\n[-] Service migration failed: %v\n", err)
		if res != nil {
			for _, step := range res.Steps {
				fmt.Printf("    [%s] %s: %s\n", step.Status, step.Phase, step.Details)
			}
		}
		return
	}

	fmt.Printf("\n[✓] Service Migration Completed Successfully in %v!\n", res.TotalDuration)
	for _, step := range res.Steps {
		fmt.Printf("    [%s] %s: %s (%v)\n", step.Status, step.Phase, step.Details, step.Duration)
	}

	if len(res.UpdatedEndpoints) > 0 {
		fmt.Println("\n[+] Updated Cluster Service Endpoints:")
		for svc, endpoint := range res.UpdatedEndpoints {
			fmt.Printf("    * %-15s -> %s\n", svc, endpoint)
		}
	}
}

func printDebianCapacityPlan(plan migrate.DebianCapacityPlan) {
	fmt.Println("==========================================================================================")
	fmt.Printf("              DENDRITE SERVICE CAPACITY EVALUATION: %s\n", strings.ToUpper(plan.ServerProfile))
	fmt.Println("==========================================================================================")
	fmt.Printf("  * Total Server RAM : %d MB (%.1f GB)\n", plan.TotalRAMMB, float64(plan.TotalRAMMB)/1024.0)
	fmt.Printf("  * Total Server Disk: %d GB (%.1f TB)\n", plan.TotalDiskGB, float64(plan.TotalDiskGB)/1000.0)
	fmt.Printf("  * OS System Reserve: 1024 MB (Debian Linux kernel, systemd, SSH daemon)\n")
	fmt.Printf("  * Net Service RAM  : %d MB (Allocated: %d MB, Remaining: %d MB)\n",
		plan.TotalRAMMB-1024, plan.AllocatedRAMMB, plan.AvailableRAMMB)
	fmt.Printf("  * Net Service Disk : %d GB (Allocated: %d GB, Remaining: %d GB)\n",
		plan.TotalDiskGB, plan.AllocatedDiskGB, plan.AvailableDiskGB)
	fmt.Println("------------------------------------------------------------------------------------------")

	fmt.Println("[+] SERVICES ELIGIBLE TO LIVE ON DEBIAN SERVER (CUMULATIVE FIT):")
	for idx, e := range plan.EligibleServices {
		fmt.Printf("  %d. %-15s [%s]\n", idx+1, e.Service, e.DisplayName)
		fmt.Printf("     Footprint: %d MB RAM, %d GB Disk\n", e.RAMMB, e.DiskGB)
		fmt.Printf("     Status   : [✓] %s\n", e.Reason)
	}

	if len(plan.ExcludedServices) > 0 {
		fmt.Println("\n[-] SERVICES EXCLUDED / RETAINED ON LOCAL WORKSTATION:")
		for idx, e := range plan.ExcludedServices {
			fmt.Printf("  %d. %-15s [%s]\n", idx+1, e.Service, e.DisplayName)
			fmt.Printf("     Footprint: %d MB RAM, %d GB Disk\n", e.RAMMB, e.DiskGB)
			fmt.Printf("     Constraint: [!] %s\n", e.Reason)
		}
	}

	fmt.Println("------------------------------------------------------------------------------------------")
	fmt.Printf("[+] RECOMMENDED EXECUTION COMMAND:\n")
	fmt.Printf("    %s\n", plan.RecommendedAction)
	fmt.Println("==========================================================================================")
}

func parseSizeToMB(raw string, defaultMB uint64) uint64 {
	clean := strings.ToUpper(strings.TrimSpace(raw))
	if strings.HasSuffix(clean, "GB") {
		numStr := strings.TrimSuffix(clean, "GB")
		if n, err := strconv.ParseUint(numStr, 10, 64); err == nil {
			return n * 1024
		}
	} else if strings.HasSuffix(clean, "MB") {
		numStr := strings.TrimSuffix(clean, "MB")
		if n, err := strconv.ParseUint(numStr, 10, 64); err == nil {
			return n
		}
	} else if n, err := strconv.ParseUint(clean, 10, 64); err == nil {
		return n
	}
	return defaultMB
}

func parseSizeToGB(raw string, defaultGB uint64) uint64 {
	clean := strings.ToUpper(strings.TrimSpace(raw))
	if strings.HasSuffix(clean, "TB") {
		numStr := strings.TrimSuffix(clean, "TB")
		if n, err := strconv.ParseUint(numStr, 10, 64); err == nil {
			return n * 1000
		}
	} else if strings.HasSuffix(clean, "GB") {
		numStr := strings.TrimSuffix(clean, "GB")
		if n, err := strconv.ParseUint(numStr, 10, 64); err == nil {
			return n
		}
	} else if n, err := strconv.ParseUint(clean, 10, 64); err == nil {
		return n
	}
	return defaultGB
}
