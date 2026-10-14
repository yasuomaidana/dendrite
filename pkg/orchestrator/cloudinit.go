package orchestrator

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultBootstrapPackages returns essential dependencies for Docker + overlay network nodes.
func DefaultBootstrapPackages() []string {
	return []string{
		"curl",
		"ca-certificates",
		"gnupg",
		"lsb-release",
		"iptables",
		"docker.io",
		"docker-compose-v2",
	}
}

// GenerateCloudInit renders a valid multi-stage cloud-init YAML script.
func GenerateCloudInit(cfg CloudInitConfig) (string, error) {
	if strings.TrimSpace(cfg.Hostname) == "" {
		return "", &OrchestrationError{Node: cfg.Hostname, Op: "GenerateCloudInit", Err: fmt.Errorf("hostname cannot be empty")}
	}

	packages := cfg.Packages
	if len(packages) == 0 {
		packages = DefaultBootstrapPackages()
	}

	// Merge packages from pluggable NetworkProvider if specified
	if cfg.NetworkProvider != nil {
		pkgSet := make(map[string]bool)
		for _, p := range packages {
			pkgSet[p] = true
		}
		for _, p := range cfg.NetworkProvider.BootstrapPackages() {
			if !pkgSet[p] {
				packages = append(packages, p)
				pkgSet[p] = true
			}
		}
	}

	users := make([]map[string]interface{}, 0)
	dockerUsers := cfg.DockerUsers
	if len(dockerUsers) == 0 {
		dockerUsers = []string{"deployer"}
	}

	for _, username := range dockerUsers {
		userMap := map[string]interface{}{
			"name":                username,
			"gecos":               "Dendrite Automated Deployer",
			"groups":              "sudo, docker",
			"sudo":                "ALL=(ALL) NOPASSWD:ALL",
			"shell":               "/bin/bash",
			"lock_passwd":         true,
			"ssh_authorized_keys": cfg.SSHAuthorizedKeys,
		}
		users = append(users, userMap)
	}

	runCmd := []string{
		"systemctl enable --now docker",
	}

	// Determine network bootstrapping commands
	if cfg.NetworkProvider != nil {
		providerCmds := cfg.NetworkProvider.BootstrapCommands(cfg.Hostname)
		runCmd = append(runCmd, providerCmds...)
	} else {
		// Default backward-compatible Tailscale bootstrap
		runCmd = append(runCmd,
			"curl -fsSL https://tailscale.com/install.sh | sh",
			"systemctl enable --now tailscaled",
		)

		if cfg.TailscaleAuthKey != "" {
			tailscaleCmd := fmt.Sprintf("tailscale up --authkey=%s --hostname=%s", cfg.TailscaleAuthKey, cfg.Hostname)
			if cfg.EnableTailscaleSSH {
				tailscaleCmd += " --ssh"
			}
			runCmd = append(runCmd, tailscaleCmd)
		}
	}

	// Append any user-provided extra commands
	if len(cfg.ExtraRunCmd) > 0 {
		runCmd = append(runCmd, cfg.ExtraRunCmd...)
	}

	cloudConfig := map[string]interface{}{
		"hostname":         cfg.Hostname,
		"manage_etc_hosts": true,
		"package_update":   true,
		"packages":         packages,
		"users":            users,
		"runcmd":           runCmd,
	}

	yamlBytes, err := yaml.Marshal(cloudConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cloud-init configuration: %w", err)
	}

	return "#cloud-config\n" + string(yamlBytes), nil
}
