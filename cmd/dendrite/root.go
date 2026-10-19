package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// commandGroups are the Neuro-Lore categories, in help order.
var commandGroups = []*cobra.Group{
	{ID: "soma", Title: "Soma & Axon Hillock:"},
	{ID: "cortex", Title: "Cortex & Sensory Integration:"},
	{ID: "effectors", Title: "Postsynaptic Effectors & Actuation:"},
	{ID: "cleft", Title: "Synaptic Cleft & Gating:"},
	{ID: "axon", Title: "Axon & Myelin Sheath:"},
	{ID: "arbor", Title: "Dendritic Arbor:"},
	{ID: "glia", Title: "Glial Regulation & Pruning:"},
	{ID: "boutons", Title: "Terminal Boutons & Vesicles:"},
}

// groupIDFor maps a category title to its group ID.
func groupIDFor(category string) string {
	for _, g := range commandGroups {
		if strings.TrimSuffix(g.Title, ":") == category {
			return g.ID
		}
	}
	return ""
}

// groupTitle maps a group ID to its category title.
func groupTitle(id string) string {
	for _, g := range commandGroups {
		if g.ID == id {
			return strings.TrimSuffix(g.Title, ":")
		}
	}
	return ""
}

// newRootCmd builds the full dendrite command tree.
func newRootCmd() *cobra.Command {
	root := newBareRootCmd()
	root.AddCommand(
		newVersionCmd(),
		newSynapseCmd(),
		newSessionCmd(),
		newBoutonCmd(),
		newSomaCmd(),
		newGanglionCmd(),
		newWorkCmd(),
		newChronosCmd(),
		newCleftCmd(),
		newSpikeCmd(),
		newVelocityCmd(),
		newInterCmd(),
		newAxonCmd(),
		newArborCmd(),
		newAfferentCmd(),
		newPlexusCmd(),
		newRetroCmd(),
		newMeshCmd(),
		newEngramCmd(),
		newCognitionCmd(),
		newVesicleCmd(),
		newQuantalCmd(),
	)
	root.AddCommand(legacyCommands()...)
	return root
}

// newBareRootCmd builds the root command (groups, global flags, error handling) without subcommands.
func newBareRootCmd() *cobra.Command {
	cobra.EnableCaseInsensitive = true

	root := &cobra.Command{
		Use:   "dendrite",
		Short: "Autonomous AI development & infrastructure control plane",
		Long: "Dendrite - Autonomous AI Development & Infrastructure Control Plane.\n\n" +
			"Commands use Neuro-Lore names; the plain names (env, plane, agent, ...) work as aliases.",
		Version:                    version,
		SilenceUsage:               true,
		SilenceErrors:              true,
		SuggestionsMinimumDistance: 2,
		Args:                       asUsage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return applyGlobals()
		},
	}
	root.SetVersionTemplate("Dendrite CLI v{{.Version}}\n")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &usageError{err: err}
	})
	bindGlobalFlags(root)
	root.AddGroup(commandGroups...)
	return root
}

// newVersionCmd prints the CLI version.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print Dendrite CLI version",
		GroupID: "cortex",
		Args:    noArgs,
		Example: "dendrite version\ndendrite --version\ndendrite -v",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Printf("Dendrite CLI v%s\n", version)
			return nil
		},
	}
}

// newGroupCmd builds a parent command: no args prints help, an unknown subcommand is a usage error.
func newGroupCmd(use, short, groupID string, aliases ...string) *cobra.Command {
	return &cobra.Command{
		Use:     use,
		Short:   short,
		GroupID: groupID,
		Aliases: aliases,
		RunE:    runGroup,
	}
}

// runGroup is the RunE of every parent command.
func runGroup(cmd *cobra.Command, args []string) error {
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		return cmd.Help()
	}
	msg := "unknown command %q for %q"
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2
	}
	if s := cmd.SuggestionsFor(args[0]); len(s) > 0 {
		msg += "\n\nDid you mean this?\n\t" + strings.Join(s, "\n\t")
	}
	return usageErrorf(msg+"\nRun '%s --help' for usage.", args[0], cmd.CommandPath(), cmd.CommandPath())
}

// legacySpec describes a command whose handler still parses its own arguments.
// It exists only until every command is migrated to native cobra subcommands (DEN-950).
type legacySpec struct {
	Name        string
	Aliases     []string
	Category    string
	Description string
	Run         func(ctx context.Context, args []string) error
}

// legacyCmd adapts a hand-parsed handler to cobra: flags pass through untouched.
func legacyCmd(s legacySpec) *cobra.Command {
	return &cobra.Command{
		Use:                s.Name,
		Aliases:            s.Aliases,
		Short:              s.Description,
		GroupID:            groupIDFor(s.Category),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return cmd.Help()
			}
			args = extractGlobalFlags(args)
			if err := applyGlobals(); err != nil {
				return err
			}
			return s.Run(cmd.Context(), args)
		},
	}
}

// legacySub adapts one subcommand of a hand-parsed handler, e.g. `bouton run` -> handleAgentCommand(ctx, "run", args).
func legacySub(name, short string, run func(ctx context.Context, args []string) error) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              short,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return cmd.Help()
			}
			args = extractGlobalFlags(args)
			if err := applyGlobals(); err != nil {
				return err
			}
			return run(cmd.Context(), args)
		},
	}
}

// extractGlobalFlags removes the value-taking global flags from raw args and records them in globals.
// --json and -y/--yes stay in args because legacy handlers parse them themselves.
func extractGlobalFlags(args []string) []string {
	targets := map[string]*string{
		"--state-dir":   &globals.StateDir,
		"--home":        &globals.Home,
		"--gateway-url": &globals.GatewayURL,
		"--plane-url":   &globals.PlaneURL,
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, value, hasValue := strings.Cut(a, "=")
		p, ok := targets[name]
		if !ok {
			switch a {
			case "--json":
				globals.JSON = true
			case "--yes", "-y":
				globals.Yes = true
			}
			out = append(out, a)
			continue
		}
		if !hasValue && i+1 < len(args) {
			value = args[i+1]
			i++
		}
		*p = value
	}
	return out
}

// invokeCmd runs cmd with args on a bare root, printing any error.
// Migrated groups use it to keep their old handle* entry points working for legacy callers
// in other files; the shims are deleted once nothing calls them (DEN-1032).
func invokeCmd(ctx context.Context, cmd *cobra.Command, args ...string) error {
	root := newBareRootCmd()
	root.AddCommand(cmd)
	root.SetArgs(append([]string{cmd.Name()}, args...))
	err := root.ExecuteContext(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[-] %v\n", err)
	}
	return err
}
