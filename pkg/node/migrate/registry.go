package migrate

import (
	"Dendrite/pkg/stacks"
	"fmt"
	"strings"
)

// DefaultServiceCatalog returns the canonical inventory of Dendrite services.
func DefaultServiceCatalog() map[string]ServiceProfile {
	return map[string]ServiceProfile{
		"plane": {
			Name:           "plane",
			DisplayName:    "Plane v2 Self-Hosted Project Management Stack",
			Description:    "Authoritative project tracking, issues, milestones, and pages (PostgreSQL 5434, Redis 6380, API 8090, UI 8091)",
			ComposeFile:    stacks.Path("compose.plane.yaml"),
			Port:           8090,
			HealthEndpoint: "/api/health/",
			MinRAMMB:       1536,
			MinDiskGB:      20,
			RequiresGPU:    false,
			Containers: []string{
				"dendrite-plane-api",
				"dendrite-plane-ui",
				"dendrite-plane-proxy",
				"dendrite-plane-db",
				"dendrite-plane-redis",
			},
			EnvKey:      "PLANE_API_URL",
			SupportedOS: []string{"linux", "debian", "ubuntu", "darwin"},
		},
		"litellm": {
			Name:           "litellm",
			DisplayName:    "LiteLLM Token Governance Gateway",
			Description:    "Unified LLM proxy with master key, spend limits, virtual keys, and fallback cascades (:4000)",
			ComposeFile:    stacks.Path("compose.control.yaml"),
			Port:           4000,
			HealthEndpoint: "/health/liveliness",
			MinRAMMB:       512,
			MinDiskGB:      5,
			RequiresGPU:    false,
			Containers: []string{
				"dendrite-litellm",
			},
			EnvKey:      "LITELLM_PROXY_URL",
			SupportedOS: []string{"linux", "debian", "ubuntu", "darwin"},
		},
		"database": {
			Name:           "database",
			DisplayName:    "PostgreSQL 16 & Redis 7 Control Plane Storage",
			Description:    "Core persistence layer for token ledgers, agent telemetry, and session state (:5432, :6379)",
			ComposeFile:    stacks.Path("compose.control.yaml"),
			Port:           5432,
			HealthEndpoint: "",
			MinRAMMB:       1024,
			MinDiskGB:      30,
			RequiresGPU:    false,
			Containers: []string{
				"dendrite-postgres",
				"dendrite-redis",
			},
			EnvKey:      "POSTGRES_HOST",
			SupportedOS: []string{"linux", "debian", "ubuntu", "darwin"},
		},
		"control": {
			Name:           "control",
			DisplayName:    "Combined Control Plane (LiteLLM + Postgres + Redis)",
			Description:    "Full Dendrite control plane stack (LiteLLM :4000, Postgres :5432, Redis :6379)",
			ComposeFile:    stacks.Path("compose.control.yaml"),
			Port:           4000,
			HealthEndpoint: "/health/liveliness",
			MinRAMMB:       1536,
			MinDiskGB:      35,
			RequiresGPU:    false,
			Containers: []string{
				"dendrite-litellm",
				"dendrite-postgres",
				"dendrite-redis",
			},
			EnvKey:      "LITELLM_PROXY_URL",
			SupportedOS: []string{"linux", "debian", "ubuntu", "darwin"},
		},
		"penpot": {
			Name:           "penpot",
			DisplayName:    "Penpot Open-Source Design Stack",
			Description:    "Design and prototyping frontend, backend, database and redis (:9000, :9001)",
			ComposeFile:    stacks.Path("compose.penpot.yaml"),
			Port:           9001,
			HealthEndpoint: "/api/health",
			MinRAMMB:       1536,
			MinDiskGB:      20,
			RequiresGPU:    false,
			Containers: []string{
				"dendrite-penpot-frontend",
				"dendrite-penpot-backend",
				"dendrite-penpot-db",
				"dendrite-penpot-redis",
			},
			EnvKey:      "PENPOT_PUBLIC_URI",
			SupportedOS: []string{"linux", "debian", "ubuntu", "darwin"},
		},
		"grafana": {
			Name:           "grafana",
			DisplayName:    "Prometheus 2.50 & Grafana 10.3 Observability",
			Description:    "Real-time SoC thermals, token spend metrics, and system dashboards (:3000, :9090)",
			ComposeFile:    stacks.Path("compose.grafana.yaml"),
			Port:           3000,
			HealthEndpoint: "/api/health",
			MinRAMMB:       512,
			MinDiskGB:      10,
			RequiresGPU:    false,
			Containers: []string{
				"dendrite-grafana",
				"dendrite-prometheus",
			},
			EnvKey:      "GRAFANA_URL",
			SupportedOS: []string{"linux", "debian", "ubuntu", "darwin"},
		},
		"ollama": {
			Name:           "ollama",
			DisplayName:    "Ollama Metal GPU Inference Engine",
			Description:    "Local zero-cost LLM inference (qwen2.5-coder:14b, llama3.2:3b). Requires Apple Silicon Metal or CUDA GPU.",
			ComposeFile:    "",
			Port:           11434,
			HealthEndpoint: "/api/tags",
			MinRAMMB:       8192,
			MinDiskGB:      50,
			RequiresGPU:    true,
			Containers:     []string{"dendrite-ollama"},
			EnvKey:         "OLLAMA_BASE_URL",
			SupportedOS:    []string{"darwin", "linux"},
		},
		"ghidra": {
			Name:           "ghidra",
			DisplayName:    "Headless Ghidra & Rizin Linux RE Worker",
			Description:    "Cleanroom binary reverse engineering analysis worker",
			ComposeFile:    stacks.Path("compose.dev.yaml"),
			Port:           8082,
			HealthEndpoint: "",
			MinRAMMB:       4096,
			MinDiskGB:      15,
			RequiresGPU:    false,
			Containers:     []string{"dendrite-ghidra"},
			EnvKey:         "",
			SupportedOS:    []string{"linux", "debian", "ubuntu"},
		},
	}
}

// EvaluateDebianServer calculates which services can live on a Debian server with given RAM and Disk.
// For example, 4096 MB RAM and 1000 GB Disk (1 TB).
func EvaluateDebianServer(totalRAMMB uint64, totalDiskGB uint64) DebianCapacityPlan {
	if totalRAMMB == 0 {
		totalRAMMB = 4096 // 4 GB
	}
	if totalDiskGB == 0 {
		totalDiskGB = 1000 // 1 TB
	}

	catalog := DefaultServiceCatalog()

	// Reserve 1024 MB for Debian Linux OS, systemd, SSH daemon, and kernel buffers
	const osReserveRAMMB uint64 = 1024
	availableForServicesRAM := totalRAMMB
	if availableForServicesRAM > osReserveRAMMB {
		availableForServicesRAM -= osReserveRAMMB
	}

	var eligible []DebianEligibility
	var excluded []DebianEligibility

	// Primary migration candidate suite for Debian server:
	// Plane (1536 MB) + LiteLLM (512 MB) + Database (1024 MB) = 3072 MB total.
	candidatePriority := []string{"plane", "litellm", "database", "grafana", "penpot", "ghidra", "ollama"}

	var allocatedRAM uint64
	var allocatedDisk uint64

	for _, name := range candidatePriority {
		svc, ok := catalog[name]
		if !ok {
			continue
		}

		// GPU filter: Debian CPU server cannot run GPU services
		if svc.RequiresGPU {
			excluded = append(excluded, DebianEligibility{
				Service:     svc.Name,
				DisplayName: svc.DisplayName,
				Eligible:    false,
				RAMMB:       svc.MinRAMMB,
				DiskGB:      svc.MinDiskGB,
				Reason:      "Requires dedicated Apple Silicon Metal or CUDA GPU; Debian CPU server lacks GPU accelerator",
			})
			continue
		}

		// Standalone memory check against total capacity
		if svc.MinRAMMB > totalRAMMB-osReserveRAMMB {
			excluded = append(excluded, DebianEligibility{
				Service:     svc.Name,
				DisplayName: svc.DisplayName,
				Eligible:    false,
				RAMMB:       svc.MinRAMMB,
				DiskGB:      svc.MinDiskGB,
				Reason:      fmt.Sprintf("Service alone requires %d MB RAM; exceeds net allocatable server RAM (%d MB after OS reserve)", svc.MinRAMMB, availableForServicesRAM),
			})
			continue
		}

		// Cumulative capacity fitting: prioritize Plane, LiteLLM, Database
		if allocatedRAM+svc.MinRAMMB <= availableForServicesRAM && allocatedDisk+svc.MinDiskGB <= totalDiskGB {
			allocatedRAM += svc.MinRAMMB
			allocatedDisk += svc.MinDiskGB
			eligible = append(eligible, DebianEligibility{
				Service:     svc.Name,
				DisplayName: svc.DisplayName,
				Eligible:    true,
				RAMMB:       svc.MinRAMMB,
				DiskGB:      svc.MinDiskGB,
				Reason:      fmt.Sprintf("Fits within Debian memory budget (Allocated: %d MB / %d MB available)", allocatedRAM, availableForServicesRAM),
			})
		} else {
			excluded = append(excluded, DebianEligibility{
				Service:     svc.Name,
				DisplayName: svc.DisplayName,
				Eligible:    false,
				RAMMB:       svc.MinRAMMB,
				DiskGB:      svc.MinDiskGB,
				Reason:      fmt.Sprintf("Exceeds cumulative memory limit with already allocated services (Would need %d MB, only %d MB remaining)", allocatedRAM+svc.MinRAMMB, availableForServicesRAM-allocatedRAM),
			})
		}
	}

	remainingRAM := totalRAMMB - allocatedRAM - osReserveRAMMB
	remainingDisk := totalDiskGB - allocatedDisk

	var eligibleNames []string
	for _, e := range eligible {
		eligibleNames = append(eligibleNames, e.Service)
	}

	recommendedCmd := fmt.Sprintf("dendrite translocate run %s --ssh user@debian-server", strings.Join(eligibleNames, " "))

	return DebianCapacityPlan{
		ServerProfile:     fmt.Sprintf("Debian Linux (RAM: %.1f GB, Disk: %d GB)", float64(totalRAMMB)/1024.0, totalDiskGB),
		TotalRAMMB:        totalRAMMB,
		TotalDiskGB:       totalDiskGB,
		AllocatedRAMMB:    allocatedRAM,
		AllocatedDiskGB:   allocatedDisk,
		AvailableRAMMB:    remainingRAM,
		AvailableDiskGB:   remainingDisk,
		EligibleServices:  eligible,
		ExcludedServices:  excluded,
		RecommendedAction: recommendedCmd,
	}
}
