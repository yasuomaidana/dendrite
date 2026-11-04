package main

import (
	"Dendrite/cmd/dendrite/internal/cli"

	"github.com/spf13/cobra"
)

// newRootCmd builds the full dendrite command tree, listed by help group.
func newRootCmd() *cobra.Command {
	root := cli.NewBareRootCmd()
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

// newVersionCmd prints the CLI version.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print Dendrite CLI version",
		GroupID: "cortex",
		Args:    cli.NoArgs,
		Example: "dendrite version\ndendrite --version\ndendrite -v",
		RunE: func(cmd *cobra.Command, args []string) error {
			if rev := buildRevision(); rev != "" {
				cmd.Printf("Dendrite CLI v%s (%s)\n", cli.Version, rev)
				return nil
			}
			cmd.Printf("Dendrite CLI v%s\n", cli.Version)
			return nil
		},
	}
}
