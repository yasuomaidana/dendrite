package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var (
	lookPath       = exec.LookPath
	commandContext = exec.CommandContext
)

// AgentInstallSpec describes the package manager command required to install an agent.
type AgentInstallSpec struct {
	Binary      string
	Command     string
	Args        []string
	Description string
}

// GetAgentInstallSpec returns the recommended installation command for the given agent on this platform.
func GetAgentInstallSpec(agentType AgentType) (AgentInstallSpec, error) {
	switch agentType {
	case AgentAider:
		// Check for Homebrew on macOS / Linux
		if _, err := lookPath("brew"); err == nil {
			return AgentInstallSpec{
				Binary:      "aider",
				Command:     "brew",
				Args:        []string{"install", "aider"},
				Description: "AI pair programming in your terminal (Homebrew formula)",
			}, nil
		}
		// Fallback to pip3
		return AgentInstallSpec{
			Binary:      "aider",
			Command:     "pip3",
			Args:        []string{"install", "--break-system-packages", "aider-chat"},
			Description: "AI pair programming in your terminal (Python package)",
		}, nil

	case AgentCopilot, AgentGitHubCopilot:
		if _, err := lookPath("brew"); err == nil {
			return AgentInstallSpec{
				Binary:      "copilot",
				Command:     "brew",
				Args:        []string{"install", "copilot-cli"},
				Description: "GitHub Copilot CLI coding assistant (Homebrew formula)",
			}, nil
		}
		return AgentInstallSpec{
			Binary:      "copilot",
			Command:     "npm",
			Args:        []string{"install", "-g", "@githubnext/github-copilot-cli"},
			Description: "GitHub Copilot CLI coding assistant (npm package)",
		}, nil

	case AgentOpenHands:
		if _, err := lookPath("pip3"); err == nil {
			return AgentInstallSpec{
				Binary:      "openhands",
				Command:     "pip3",
				Args:        []string{"install", "--break-system-packages", "openhands-ai"},
				Description: "OpenHands autonomous software engineering agent (Python package)",
			}, nil
		}
		return AgentInstallSpec{
			Binary:      "openhands",
			Command:     "docker",
			Args:        []string{"pull", "docker.all-hands.dev/all-hands-ai/runtime:0.14-nikolaik"},
			Description: "OpenHands container runtime sandbox",
		}, nil

	case AgentOpenCode:
		return AgentInstallSpec{
			Binary:      "opencode",
			Command:     "npm",
			Args:        []string{"install", "-g", "opencode-ai"},
			Description: "OpenCode automated refactoring agent (npm package)",
		}, nil

	case AgentAGY, AgentAntigravity:
		return AgentInstallSpec{
			Binary:      "agy",
			Command:     "curl",
			Args:        []string{"-fsSL", "https://antigravity.google.com/install.sh"},
			Description: "Google Antigravity autonomous coding agent CLI",
		}, nil

	default:
		return AgentInstallSpec{}, fmt.Errorf("no installation spec available for agent type %q", agentType)
	}
}

// PromptAndInstallAgent handles the user interaction workflow for installing missing agents.
// If autoApprove is false, it prompts the user to allow running the installation command.
func PromptAndInstallAgent(ctx context.Context, agentType AgentType, autoApprove bool, in io.Reader, out io.Writer) error {
	spec, err := GetAgentInstallSpec(agentType)
	if err != nil {
		return err
	}

	// Check if already installed
	if _, err := lookPath(spec.Binary); err == nil {
		return nil
	}

	cmdStr := fmt.Sprintf("%s %s", spec.Command, strings.Join(spec.Args, " "))

	if out == nil {
		out = os.Stdout
	}
	if in == nil {
		in = os.Stdin
	}

	_, _ = fmt.Fprintf(out, "[-] Agent executable %q is not installed in your PATH.\n", spec.Binary)
	_, _ = fmt.Fprintf(out, "[+] Description: %s\n", spec.Description)
	_, _ = fmt.Fprintf(out, "[+] Workflow Command: `%s`\n", cmdStr)

	if !autoApprove {
		_, _ = fmt.Fprintf(out, "[?] Do you allow Dendrite to execute `%s` to install %s? [y/N]: ", cmdStr, agentType)
		reader := bufio.NewReader(in)
		response, readErr := reader.ReadString('\n')
		if readErr != nil && len(response) == 0 {
			_, _ = fmt.Fprintln(out, "\n[-] No response received. Installation aborted.")
			return fmt.Errorf("user declined installation of %s (`%s`)", agentType, cmdStr)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			_, _ = fmt.Fprintf(out, "[-] Installation cancelled by user.\n[*] To install manually, please run:\n    %s\n", cmdStr)
			return fmt.Errorf("user declined installation of %s (`%s`)", agentType, cmdStr)
		}
	} else {
		_, _ = fmt.Fprintf(out, "[+] Auto-approving installation: `%s`\n", cmdStr)
	}

	_, _ = fmt.Fprintf(out, "[+] Executing: %s ...\n", cmdStr)
	installCmd := commandContext(ctx, spec.Command, spec.Args...)
	installCmd.Stdout = out
	installCmd.Stderr = out
	installCmd.Stdin = in

	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("failed to install %s via `%s`: %w", agentType, cmdStr, err)
	}

	// Verify binary is now available
	if _, err := lookPath(spec.Binary); err != nil {
		_, _ = fmt.Fprintf(out, "[!] `%s` finished, but %q is not yet in current PATH. You may need to restart your terminal.\n", cmdStr, spec.Binary)
		return nil
	}

	_, _ = fmt.Fprintf(out, "[✓] %s successfully installed and ready for execution!\n", agentType)
	return nil
}
