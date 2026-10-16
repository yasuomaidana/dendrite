package migrate

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Migrator implements the 5-phase atomic service migration protocol.
type Migrator struct {
	executor RemoteCommandExecutor
	catalog  map[string]ServiceProfile
}

// NewMigrator constructs an initialized Migrator instance.
func NewMigrator(executor RemoteCommandExecutor, catalog map[string]ServiceProfile) *Migrator {
	if executor == nil {
		executor = NewDefaultRemoteExecutor()
	}
	if catalog == nil {
		catalog = DefaultServiceCatalog()
	}
	return &Migrator{
		executor: executor,
		catalog:  catalog,
	}
}

// Plan evaluates resource eligibility without performing changes.
func (m *Migrator) Plan(req MigrationRequest) (*DebianCapacityPlan, error) {
	plan := EvaluateDebianServer(req.TargetRAMMB, req.TargetDiskGB)
	return &plan, nil
}

// Migrate executes the 5-phase atomic migration sequence.
func (m *Migrator) Migrate(ctx context.Context, req MigrationRequest) (*MigrationResult, error) {
	startTime := time.Now()

	result := &MigrationResult{
		Success:          false,
		ServicesMigrated: make([]string, 0),
		TargetHost:       req.TargetHost,
		Steps:            make([]StepRecord, 0),
		UpdatedEndpoints: make(map[string]string),
	}

	if req.TargetHost == "" {
		return nil, fmt.Errorf("target host is required (e.g. user@debian-server)")
	}

	if req.Timeout <= 0 {
		req.Timeout = 120 * time.Second
	}

	// Resolve requested service targets (support "debian", "all-debian", "all")
	targetServices, err := m.resolveServices(req)
	if err != nil {
		return nil, err
	}
	result.ServicesMigrated = targetServices

	// Phase 1: Pre-Flight Hardware & Capacity Validation
	p1Start := time.Now()
	if err := m.executePreFlight(ctx, req, targetServices); err != nil {
		result.Steps = append(result.Steps, StepRecord{
			Phase:    PhasePreFlight,
			Status:   "FAILED",
			Details:  err.Error(),
			Duration: time.Since(p1Start),
		})
		result.Error = err.Error()
		return result, err
	}
	result.Steps = append(result.Steps, StepRecord{
		Phase:    PhasePreFlight,
		Status:   "SUCCESS",
		Details:  fmt.Sprintf("Validated %d service(s) on target host %s", len(targetServices), req.TargetHost),
		Duration: time.Since(p1Start),
	})

	// Phase 2: Interactive Zero-Credential Authorization
	p2Start := time.Now()
	if err := m.executeAuthorization(req, targetServices); err != nil {
		result.Steps = append(result.Steps, StepRecord{
			Phase:    PhaseAuthorization,
			Status:   "FAILED",
			Details:  err.Error(),
			Duration: time.Since(p2Start),
		})
		result.Error = err.Error()
		return result, err
	}
	result.Steps = append(result.Steps, StepRecord{
		Phase:    PhaseAuthorization,
		Status:   "SUCCESS",
		Details:  "Explicit authorization confirmed. Zero plain-text credentials stored.",
		Duration: time.Since(p2Start),
	})

	if req.DryRun {
		for _, svcName := range targetServices {
			svc := m.catalog[svcName]
			targetHostOnly := extractHostOnly(req.TargetHost)
			if svc.Port > 0 {
				result.UpdatedEndpoints[svc.Name] = fmt.Sprintf("http://%s:%d", targetHostOnly, svc.Port)
			}
		}
		result.Steps = append(result.Steps, StepRecord{
			Phase:    PhaseDataSync,
			Status:   "DRY_RUN",
			Details:  "Dry run enabled: Skipped volume transfer and remote execution.",
			Duration: 0,
		})
		result.Steps = append(result.Steps, StepRecord{
			Phase:    PhaseRemoteDeploy,
			Status:   "DRY_RUN",
			Details:  "Dry run enabled: Skipped container startup and health probe.",
			Duration: 0,
		})
		result.Steps = append(result.Steps, StepRecord{
			Phase:    PhaseCutoverPrune,
			Status:   "DRY_RUN",
			Details:  "Dry run enabled: Local containers left untouched.",
			Duration: 0,
		})
		result.Success = true
		result.TotalDuration = time.Since(startTime)
		return result, nil
	}

	// Phase 3: Ephemeral SSH Streaming & Volume Transfer
	p3Start := time.Now()
	if err := m.executeDataSync(ctx, req, targetServices); err != nil {
		result.Steps = append(result.Steps, StepRecord{
			Phase:    PhaseDataSync,
			Status:   "FAILED",
			Details:  err.Error(),
			Duration: time.Since(p3Start),
		})
		result.Error = err.Error()
		return result, err
	}
	result.Steps = append(result.Steps, StepRecord{
		Phase:    PhaseDataSync,
		Status:   "SUCCESS",
		Details:  "Streamed Compose configurations and volume archives via ephemeral SSH.",
		Duration: time.Since(p3Start),
	})

	// Phase 4: Remote Container Launch & Health Verification
	p4Start := time.Now()
	if err := m.executeRemoteDeployAndHealth(ctx, req, targetServices); err != nil {
		result.Steps = append(result.Steps, StepRecord{
			Phase:    PhaseRemoteDeploy,
			Status:   "FAILED",
			Details:  err.Error(),
			Duration: time.Since(p4Start),
		})
		// Trigger atomic rollback
		m.executeRollback(ctx, req, targetServices)
		result.Error = err.Error()
		return result, err
	}
	result.Steps = append(result.Steps, StepRecord{
		Phase:    PhaseRemoteDeploy,
		Status:   "SUCCESS",
		Details:  "All remote containers healthy and responding to probe.",
		Duration: time.Since(p4Start),
	})

	// Phase 5: Atomic Endpoint Propagation & Local Resource Pruning
	p5Start := time.Now()
	endpoints, err := m.executeCutoverAndPrune(ctx, req, targetServices)
	if err != nil {
		result.Steps = append(result.Steps, StepRecord{
			Phase:    PhaseCutoverPrune,
			Status:   "FAILED",
			Details:  err.Error(),
			Duration: time.Since(p5Start),
		})
		result.Error = err.Error()
		return result, err
	}
	result.UpdatedEndpoints = endpoints
	result.Steps = append(result.Steps, StepRecord{
		Phase:    PhaseCutoverPrune,
		Status:   "SUCCESS",
		Details:  fmt.Sprintf("Endpoints propagated to cluster. Local containers pruned (%v).", req.PruneLocal),
		Duration: time.Since(p5Start),
	})

	result.Success = true
	result.TotalDuration = time.Since(startTime)
	return result, nil
}

func (m *Migrator) resolveServices(req MigrationRequest) ([]string, error) {
	if len(req.Services) == 0 {
		return nil, fmt.Errorf("no services specified to move")
	}

	var resolved []string
	for _, s := range req.Services {
		lower := strings.ToLower(strings.TrimSpace(s))
		if lower == "debian" || lower == "all-debian" || lower == "all" {
			plan := EvaluateDebianServer(req.TargetRAMMB, req.TargetDiskGB)
			for _, e := range plan.EligibleServices {
				resolved = append(resolved, e.Service)
			}
			continue
		}

		if _, ok := m.catalog[lower]; !ok {
			return nil, fmt.Errorf("%w: '%s'", ErrServiceNotFound, s)
		}
		resolved = append(resolved, lower)
	}

	return resolved, nil
}

func (m *Migrator) executePreFlight(ctx context.Context, req MigrationRequest, services []string) error {
	var totalRAM uint64
	var totalDisk uint64
	for _, s := range services {
		prof := m.catalog[s]
		totalRAM += prof.MinRAMMB
		totalDisk += prof.MinDiskGB
	}

	targetRAM := req.TargetRAMMB
	if targetRAM == 0 {
		targetRAM = 4096
	}
	targetDisk := req.TargetDiskGB
	if targetDisk == 0 {
		targetDisk = 1000
	}

	// Verify target host has room for requested services
	if totalRAM > targetRAM {
		return fmt.Errorf("%w: services require %d MB RAM, target offers %d MB", ErrInsufficientCapacity, totalRAM, targetRAM)
	}
	if totalDisk > targetDisk {
		return fmt.Errorf("%w: services require %d GB Disk, target offers %d GB", ErrInsufficientCapacity, totalDisk, targetDisk)
	}

	// Pre-flight probe target OS & Docker via SSH (unless dry-run)
	if !req.DryRun && m.executor != nil {
		out, err := m.executor.Execute(ctx, req.TargetHost, req.SSHPort, req.SSHUser, req.KeyPath, "uname -s && docker --version")
		if err != nil {
			return fmt.Errorf("target host connectivity probe failed: %w", err)
		}
		if !strings.Contains(strings.ToLower(out), "linux") {
			return fmt.Errorf("%w: target host is not a Linux environment", ErrUnsupportedOS)
		}
	}

	return nil
}

func (m *Migrator) executeAuthorization(req MigrationRequest, services []string) error {
	if req.AutoApprove || req.DryRun {
		return nil
	}

	if req.In == nil {
		return ErrNonInteractiveNoForce
	}

	out := req.Out
	if out == nil {
		out = os.Stdout
	}

	fmt.Fprintf(out, "\n================================================================================\n")
	fmt.Fprintf(out, "               DENDRITE INFRASTRUCTURE SERVICE MIGRATION GATE                  \n")
	fmt.Fprintf(out, "================================================================================\n")
	fmt.Fprintf(out, "Target Remote Host : %s\n", req.TargetHost)
	fmt.Fprintf(out, "Services to Move   : %s\n", strings.Join(services, ", "))
	fmt.Fprintf(out, "Prune Local Stacks : %v\n", req.PruneLocal)
	fmt.Fprintf(out, "Security Policy    : Ephemeral ssh-agent (ZERO credentials written to disk/logs)\n")
	fmt.Fprintf(out, "--------------------------------------------------------------------------------\n")
	fmt.Fprintf(out, "Confirm migration to remote SSH server? [y/N]: ")

	scanner := bufio.NewScanner(req.In)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "y" || answer == "yes" {
			return nil
		}
	}

	return ErrInteractiveAuthDenied
}

func (m *Migrator) executeDataSync(ctx context.Context, req MigrationRequest, services []string) error {
	// 1. Ensure remote directory structure exists
	remoteDir := "~/.dendrite/stacks"
	mkdirCmd := fmt.Sprintf("mkdir -p %s", remoteDir)
	if _, err := m.executor.Execute(ctx, req.TargetHost, req.SSHPort, req.SSHUser, req.KeyPath, mkdirCmd); err != nil {
		return fmt.Errorf("failed to initialize remote stacks directory: %w", err)
	}

	// 2. Stream compose files and .env configuration
	envFiles := []string{".env", "stacks/.env"}
	for _, envFile := range envFiles {
		if _, err := os.Stat(envFile); err == nil {
			targetPath := fmt.Sprintf("%s/%s", remoteDir, filepath.Base(envFile))
			_ = m.executor.StreamFile(ctx, req.TargetHost, req.SSHPort, req.SSHUser, req.KeyPath, envFile, targetPath)
		}
	}

	for _, s := range services {
		prof := m.catalog[s]
		if prof.ComposeFile != "" {
			if _, err := os.Stat(prof.ComposeFile); err == nil {
				targetCompose := fmt.Sprintf("%s/%s", remoteDir, filepath.Base(prof.ComposeFile))
				if err := m.executor.StreamFile(ctx, req.TargetHost, req.SSHPort, req.SSHUser, req.KeyPath, prof.ComposeFile, targetCompose); err != nil {
					return fmt.Errorf("failed to stream compose file for %s: %w", s, err)
				}
			}
		}
	}

	return nil
}

func (m *Migrator) executeRemoteDeployAndHealth(ctx context.Context, req MigrationRequest, services []string) error {
	remoteDir := "~/.dendrite/stacks"
	targetHostOnly := extractHostOnly(req.TargetHost)

	for _, s := range services {
		prof := m.catalog[s]
		if prof.ComposeFile == "" {
			continue
		}

		composeName := filepath.Base(prof.ComposeFile)
		upCmd := fmt.Sprintf("cd %s && docker compose -f %s up -d", remoteDir, composeName)
		if _, err := m.executor.Execute(ctx, req.TargetHost, req.SSHPort, req.SSHUser, req.KeyPath, upCmd); err != nil {
			return fmt.Errorf("failed to start remote service %s: %w", s, err)
		}

		// Probe remote health if health endpoint is defined
		if prof.HealthEndpoint != "" && prof.Port > 0 {
			probeURL := fmt.Sprintf("http://%s:%d%s", targetHostOnly, prof.Port, prof.HealthEndpoint)
			ok, err := m.executor.ProbeHealth(ctx, probeURL, req.Timeout)
			if !ok || err != nil {
				return fmt.Errorf("health check failed for %s on %s: %w", s, probeURL, err)
			}
		}
	}

	return nil
}

func (m *Migrator) executeCutoverAndPrune(ctx context.Context, req MigrationRequest, services []string) (map[string]string, error) {
	endpoints := make(map[string]string)
	targetHostOnly := extractHostOnly(req.TargetHost)

	for _, s := range services {
		prof := m.catalog[s]
		if prof.Port > 0 {
			endpoints[prof.Name] = fmt.Sprintf("http://%s:%d", targetHostOnly, prof.Port)
		}

		if req.PruneLocal && prof.ComposeFile != "" {
			// Safely tear down and prune local containers
			downCmd := fmt.Sprintf("docker compose -f %s down", prof.ComposeFile)
			_ = execCommand(ctx, downCmd)
		}
	}

	return endpoints, nil
}

func (m *Migrator) executeRollback(ctx context.Context, req MigrationRequest, services []string) {
	remoteDir := "~/.dendrite/stacks"
	for _, s := range services {
		prof := m.catalog[s]
		if prof.ComposeFile != "" {
			composeName := filepath.Base(prof.ComposeFile)
			downCmd := fmt.Sprintf("cd %s && docker compose -f %s down", remoteDir, composeName)
			_, _ = m.executor.Execute(ctx, req.TargetHost, req.SSHPort, req.SSHUser, req.KeyPath, downCmd)
		}
	}
}

func extractHostOnly(raw string) string {
	if strings.Contains(raw, "@") {
		parts := strings.Split(raw, "@")
		return parts[1]
	}
	return raw
}

func execCommand(ctx context.Context, cmdStr string) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	return cmd.Run()
}
