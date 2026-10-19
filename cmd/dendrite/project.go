package main

import (
	"fmt"
	"io"
	"time"

	"Dendrite/pkg/workspace"

	"github.com/spf13/cobra"
)

// newProjectRegistry is a seam so tests can point the CLI at a temporary registry.
var newProjectRegistry = func() (*workspace.Registry, error) { return workspace.NewRegistry("", "") }

// newGanglionCmd builds `dendrite ganglion` (alias project): the multi-project workspace registry.
func newGanglionCmd() *cobra.Command {
	cmd := newGroupCmd("ganglion", "Manage multi-project isolated workspaces", "soma",
		"project", "ganglion-workspace", "prj")
	cmd.Example = "dendrite ganglion add dendrite ~/GoLandProjects/Dendrite\n" +
		"dendrite ganglion switch dendrite\n" +
		"dendrite ganglion list\n" +
		"dendrite ganglion remove old-project --purge"

	add := &cobra.Command{
		Use:   "add <name> <path>",
		Short: "Register a project and its isolated data directory",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withRegistry(func(reg *workspace.Registry) error {
				return runGanglionAdd(reg, args[0], args[1], cmd.OutOrStdout())
			})
		},
	}
	switchCmd := &cobra.Command{
		Use:   "switch <name>",
		Short: "Make a registered project the active context",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withRegistry(func(reg *workspace.Registry) error {
				return runGanglionSwitch(reg, args[0], cmd.OutOrStdout())
			})
		},
	}
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered projects (* marks the active one)",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withRegistry(func(reg *workspace.Registry) error {
				return runGanglionList(reg, cmd.OutOrStdout())
			})
		},
	}
	var purge bool
	remove := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Unregister a project",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withRegistry(func(reg *workspace.Registry) error {
				return runGanglionRemove(reg, args[0], purge, cmd.OutOrStdout())
			})
		},
	}
	remove.Flags().BoolVar(&purge, "purge", false, "Also delete the project's data directory")

	cmd.AddCommand(add, switchCmd, list, remove)
	return cmd
}

func withRegistry(fn func(*workspace.Registry) error) error {
	reg, err := newProjectRegistry()
	if err != nil {
		return fmt.Errorf("open workspace registry: %w", err)
	}
	return fn(reg)
}

func runGanglionAdd(reg *workspace.Registry, name, path string, out io.Writer) error {
	entry, err := reg.AddProject(name, path)
	if err != nil {
		return fmt.Errorf("register project: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, entry)
	}
	fmt.Fprintf(out, "[✓] Project '%s' registered successfully (Virtual Key: %s)\n", entry.Name, entry.VirtualKeyAlias)
	fmt.Fprintf(out, "    - Code Path:   %s\n", entry.Path)
	fmt.Fprintf(out, "    - Data Dir:    %s\n", entry.DataDir)
	fmt.Fprintf(out, "    - Graph DB:    %s\n", entry.GraphDBPath)
	fmt.Fprintf(out, "    - Spend Cache: %s\n", entry.SpendCachePath)
	return nil
}

func runGanglionSwitch(reg *workspace.Registry, name string, out io.Writer) error {
	start := time.Now()
	entry, err := reg.SwitchProject(name)
	if err != nil {
		return fmt.Errorf("switch project: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, entry)
	}
	fmt.Fprintf(out, "[✓] Switched active context to '%s' in %v\n", entry.Name, time.Since(start))
	fmt.Fprintf(out, "    - Virtual Key Alias: %s\n", entry.VirtualKeyAlias)
	fmt.Fprintf(out, "    - Active Workspace:  %s\n", entry.Path)
	return nil
}

func runGanglionList(reg *workspace.Registry, out io.Writer) error {
	projects := reg.ListProjects()
	if globals.JSON {
		if projects == nil {
			projects = []workspace.ProjectEntry{}
		}
		return writeJSON(out, projects)
	}
	activeName := ""
	if active, _ := reg.GetActiveProject(); active != nil {
		activeName = active.Name
	}
	fmt.Fprintf(out, "[+] Registered Workspaces (%d):\n", len(projects))
	for _, p := range projects {
		mark := " "
		if p.Name == activeName {
			mark = "*"
		}
		fmt.Fprintf(out, "  [%s] %-15s [%s] -> %s (Key: %s)\n", mark, p.Name, p.State, p.Path, p.VirtualKeyAlias)
	}
	return nil
}

func runGanglionRemove(reg *workspace.Registry, name string, purge bool, out io.Writer) error {
	if err := reg.RemoveProject(name, purge); err != nil {
		return fmt.Errorf("remove project: %w", err)
	}
	fmt.Fprintf(out, "[✓] Project '%s' removed (Purge Data: %v)\n", name, purge)
	return nil
}
