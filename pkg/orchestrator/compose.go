package orchestrator

import (
	"fmt"
	"sort"
	"strings"
)

// NewComposeCommand initializes a ComposeCommand with defaults.
func NewComposeCommand(action string) *ComposeCommand {
	return &ComposeCommand{
		Binary:       "docker",
		ComposeFiles: make([]string, 0),
		Profiles:     make([]string, 0),
		Action:       action,
		Detached:     true,
		Environment:  make(map[string]string),
		ExtraArgs:    make([]string, 0),
	}
}

// BuildArgs constructs the slice of CLI arguments for docker compose.
func (c *ComposeCommand) BuildArgs() ([]string, error) {
	if c == nil {
		return nil, fmt.Errorf("compose command is nil")
	}

	trimmedAction := strings.TrimSpace(c.Action)
	if trimmedAction == "" {
		return nil, ErrEmptyAction
	}

	args := []string{"compose"}

	for _, file := range c.ComposeFiles {
		if file != "" {
			args = append(args, "-f", file)
		}
	}

	// Sort profiles deterministically
	sortedProfiles := make([]string, len(c.Profiles))
	copy(sortedProfiles, c.Profiles)
	sort.Strings(sortedProfiles)

	for _, p := range sortedProfiles {
		if p != "" {
			args = append(args, "--profile", p)
		}
	}

	args = append(args, trimmedAction)

	if trimmedAction == "up" && c.Detached {
		args = append(args, "-d")
	}

	if len(c.ExtraArgs) > 0 {
		args = append(args, c.ExtraArgs...)
	}

	return args, nil
}

// String formats the complete command line invocation for logging or execution.
func (c *ComposeCommand) String() string {
	if c == nil {
		return ""
	}
	binary := c.Binary
	if binary == "" {
		binary = "docker"
	}

	args, err := c.BuildArgs()
	if err != nil {
		return fmt.Sprintf("%s (invalid: %v)", binary, err)
	}

	parts := append([]string{binary}, args...)
	return strings.Join(parts, " ")
}
