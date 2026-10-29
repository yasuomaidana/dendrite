package main

import (
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

// newRootCmd builds the full dendrite command tree, listed by help group.
func newRootCmd() *cobra.Command {
	root := newBareRootCmd()
	root.AddCommand(
		// Soma & Axon Hillock
		newSomaCmd(), newGanglionCmd(), newWorkCmd(), newPlexusCmd(), newEngramCmd(),
		// Cortex & Sensory Integration
		newCortexCmd(), newSensoryCmd(), newHookCmd(), newCodexCmd(), newAskCmd(), newRagCmd(), newVersionCmd(), newUpdateCmd(), newInstallCmd(),
		// Postsynaptic Effectors & Actuation
		newBoutonCmd(), newReflexCmd(), newTriageCmd(), newMotorCmd(), newSagaCmd(),
		// Synaptic Cleft & Gating
		newCleftCmd(), newSpikeCmd(), newInterCmd(), newBridgeCmd(), newSynapseCmd(), newSessionCmd(), newGreetingCmd(),
		// Axon & Myelin Sheath
		newAxonCmd(), newMyelinCmd(), newVelocityCmd(), newMeshCmd(), newTranslocateCmd(),
		// Dendritic Arbor
		newArborCmd(), newAfferentCmd(), newRetroCmd(),
		// Glial Regulation & Pruning
		newCognitionCmd(), newPruneCmd(), newAllostasisCmd(), newDirectiveCmd(),
		// Terminal Boutons & Vesicles
		newChronosCmd(), newVesicleCmd(), newQuantalCmd(), newSyncCmd(),
	)
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
			if rev := buildRevision(); rev != "" {
				cmd.Printf("Dendrite CLI v%s (%s)\n", version, rev)
				return nil
			}
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
