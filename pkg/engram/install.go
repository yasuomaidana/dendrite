package engram

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Install makes Engram available to the agents on this machine, and is safe to run again:
//
//  1. brew installs the binary when it is missing (after a prompt unless yes).
//  2. Claude Code gets the MCP server only (`engram setup claude-code --mcp-only`). Engram's own Claude plugin
//     (hooks, scripts, skill) is NOT installed: SkillSpector scores it 71, which the immune gate quarantines
//     (DEN-427 decision). Dendrite injects the memory instead (job prompts, session resume).
//  3. Antigravity gets Engram's own setup when `agy` is installed; a failure there is a warning.
//
// Setup commands run without stdin, so Engram never asks to widen a permission allowlist.
func Install(ctx context.Context, in io.Reader, out io.Writer, yes bool) error {
	bin, err := Locate()
	if errors.Is(err, ErrMissing) {
		if bin, err = brewInstall(ctx, in, out, yes); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if v, err := Version(ctx, bin); err == nil {
		fmt.Fprintf(out, "[✓] %s at %s\n", v, bin)
	}
	for _, agent := range []struct{ tool, slug string }{{"claude", "claude-code"}, {"agy", "antigravity-cli"}} {
		if err := setupAgent(ctx, bin, out, agent.tool, agent.slug); err != nil {
			return err
		}
	}
	if h, err := Doctor(ctx, bin); err == nil {
		fmt.Fprintf(out, "[✓] engram doctor: %s (%d/%d checks ok)\n", h.Status, h.OK, h.Total)
	}
	return nil
}

// setupAgent registers Engram with one agent when its CLI is installed. Claude Code gets the MCP server only
// (no plugin); a failed Claude Code setup is an error, a failed Antigravity setup a warning.
func setupAgent(ctx context.Context, bin string, out io.Writer, tool, slug string) error {
	if _, err := exec.LookPath(tool); err != nil {
		fmt.Fprintf(out, "[i] %s not installed: skipped the %s setup\n", tool, slug)
		return nil
	}
	args := []string{"setup", slug}
	if slug == "claude-code" {
		args = append(args, "--mcp-only")
	}
	msg, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	switch {
	case err == nil:
		fmt.Fprintf(out, "[✓] Engram MCP registered for %s\n", slug)
	case slug == "claude-code":
		return fmt.Errorf("engram %s: %w\n%s", strings.Join(args, " "), err, msg)
	default:
		fmt.Fprintf(out, "[!] engram %s failed (%v); run it by hand\n", strings.Join(args, " "), err)
	}
	return nil
}

// brewInstall installs the formula and returns the binary's path.
func brewInstall(ctx context.Context, in io.Reader, out io.Writer, yes bool) (string, error) {
	brew, err := exec.LookPath("brew")
	if err != nil {
		return "", fmt.Errorf("%w (Homebrew is not installed either)", ErrMissing)
	}
	cmdStr := "brew install " + BrewFormula
	if !yes {
		fmt.Fprintf(out, "[?] Engram is not installed. Run `%s`? [y/N]: ", cmdStr)
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			return "", fmt.Errorf("engram install declined; run `%s` yourself", cmdStr)
		}
	}
	cmd := exec.CommandContext(ctx, brew, "install", BrewFormula)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", cmdStr, err)
	}
	return Locate()
}
