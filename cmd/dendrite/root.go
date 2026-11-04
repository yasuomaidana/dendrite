package main

import (
	"Dendrite/cmd/dendrite/agent"
	"Dendrite/cmd/dendrite/analysis"
	"Dendrite/cmd/dendrite/bridge"
	"Dendrite/cmd/dendrite/chronos"
	"Dendrite/cmd/dendrite/commit"
	"Dendrite/cmd/dendrite/cortex"
	"Dendrite/cmd/dendrite/env"
	"Dendrite/cmd/dendrite/hook"
	"Dendrite/cmd/dendrite/inference"
	"Dendrite/cmd/dendrite/install"
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/cmd/dendrite/knowledge"
	"Dendrite/cmd/dendrite/mesh"
	"Dendrite/cmd/dendrite/quality"
	"Dendrite/cmd/dendrite/session"
	"Dendrite/cmd/dendrite/soma"
	"Dendrite/cmd/dendrite/synapse"

	"github.com/spf13/cobra"
)

// newRootCmd builds the full dendrite command tree, listed by help group.
func newRootCmd() *cobra.Command {
	root := cli.NewBareRootCmd()
	root.AddCommand(
		// Soma & Axon Hillock
		env.NewSomaCmd(), soma.NewGanglionCmd(), soma.NewWorkCmd(), soma.NewPlexusCmd(), soma.NewEngramCmd(),
		// Cortex & Sensory Integration
		cortex.NewCortexCmd(), hook.NewSensoryCmd(), hook.NewHookCmd(), cortex.NewCodexCmd(), knowledge.NewAskCmd(), knowledge.NewRagCmd(), newVersionCmd(), install.NewUpdateCmd(), install.NewInstallCmd(),
		// Postsynaptic Effectors & Actuation
		agent.NewBoutonCmd(), analysis.NewReflexCmd(), agent.NewTriageCmd(), env.NewMotorCmd(), analysis.NewSagaCmd(),
		// Synaptic Cleft & Gating
		mesh.NewCleftCmd(), inference.NewSpikeCmd(), bridge.NewInterCmd(), bridge.NewBridgeCmd(), synapse.NewSynapseCmd(), session.NewSessionCmd(), cortex.NewGreetingCmd(),
		// Axon & Myelin Sheath
		mesh.NewAxonCmd(), soma.NewBranchCmd(), commit.NewMyelinCmd(), mesh.NewVelocityCmd(), mesh.NewMeshCmd(), mesh.NewTranslocateCmd(),
		// Dendritic Arbor
		knowledge.NewArborCmd(), knowledge.NewAfferentCmd(), bridge.NewRetroCmd(),
		// Glial Regulation & Pruning
		analysis.NewCognitionCmd(), analysis.NewPruneCmd(), quality.NewAllostasisCmd(), analysis.NewDirectiveCmd(),
		// Terminal Boutons & Vesicles
		chronos.NewChronosCmd(), knowledge.NewVesicleCmd(), commit.NewQuantalCmd(), mesh.NewSyncCmd(),
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
			if rev := install.BuildRevision(); rev != "" {
				cmd.Printf("Dendrite CLI v%s (%s)\n", cli.Version, rev)
				return nil
			}
			cmd.Printf("Dendrite CLI v%s\n", cli.Version)
			return nil
		},
	}
}

func init() { cli.NewRoot = newRootCmd }
