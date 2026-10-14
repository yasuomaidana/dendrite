package orchestrator

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultBootstrapPackages returns essential dependencies for Docker + Tailscale nodes.
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

	// Install Tailscale repository & package
	runCmd = append(runCmd,
		"curl -fsSL https://tailscale.com/install.sh | sh",
		"systemctl enable --now tailscaled",
	)

	// Tailscale join command
	if cfg.TailscaleAuthKey != "" {
		tailscaleCmd := fmt.Sprintf("tailscale up --authkey=%s --hostname=%s", cfg.TailscaleAuthKey, cfg.Hostname)
		if cfg.EnableTailscaleSSH {
			tailscaleCmd += " --ssh"
		}
		runCmd = append(runCmd, tailscaleCmd)
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
