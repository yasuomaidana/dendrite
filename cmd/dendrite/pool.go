package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"Dendrite/pkg/pool/dag"
	"Dendrite/pkg/pool/split"
	"Dendrite/pkg/pool/switcher"
	"Dendrite/pkg/ui/modal"

	"github.com/spf13/cobra"
)

// newPoolManager loads .dendrite/pool.json on use, so --state-dir is honored.
func newPoolManager() *dag.PoolManager { return dag.NewPoolManager("") }

// newPlexusCmd builds `dendrite plexus` (alias pool): the multi-directory project pool and its dependency DAG.
func newPlexusCmd() *cobra.Command {
	cmd := newGroupCmd("plexus", "Multi-directory dependency inference & topological DAG solver", "soma",
		"pool", "connectome-pool", "plexus-pool", "connectome", "plx")
	cmd.Example = "dendrite plexus add ../core ../api\n" +
		"dendrite plexus resolve --json\n" +
		"dendrite plexus switch core-lib --in ../api --mode local --local-path ../core\n" +
		"dendrite plexus split --module pkg/auth --name auth-svc"
	cmd.AddCommand(
		newPlexusAddCmd(),
		newPlexusRemoveCmd(),
		newPlexusListCmd(),
		newPlexusResolveCmd(),
		newPlexusSwitchCmd(),
		newPlexusSplitCmd(),
	)
	return cmd
}

func joinEcosystems(ecos []dag.Ecosystem) string {
	names := make([]string, 0, len(ecos))
	for _, e := range ecos {
		names = append(names, string(e))
	}
	return strings.Join(names, ", ")
}

func newPlexusAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <dir>...",
		Short: "Register project directories in the pool",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pm := newPoolManager()
			added, err := pm.Add(args...)
			if err != nil {
				return fmt.Errorf("add to pool: %w", err)
			}
			if err := pm.Save(); err != nil {
				return fmt.Errorf("save pool: %w", err)
			}
			out := cmd.OutOrStdout()
			if globals.JSON {
				return writeJSON(out, added)
			}
			fmt.Fprintf(out, "[+] Successfully registered %d project(s) in pool:\n", len(added))
			for _, p := range added {
				ecoStr := joinEcosystems(p.Ecosystems)
				if ecoStr == "" {
					ecoStr = "none"
				}
				fmt.Fprintf(out, "    * %-25s | Ecosystems: %-15s | Path: %s\n", p.Name, ecoStr, p.Path)
				if len(p.Dependencies) > 0 {
					fmt.Fprintf(out, "      Declared Dependencies (%d): %s\n", len(p.Dependencies), strings.Join(p.Dependencies, ", "))
				}
			}
			return nil
		},
	}
}

func newPlexusRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name|path>",
		Aliases: []string{"rm"},
		Short:   "Remove a project from the pool",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pm := newPoolManager()
			if err := pm.Remove(args[0]); err != nil {
				return fmt.Errorf("remove from pool: %w", err)
			}
			if err := pm.Save(); err != nil {
				return fmt.Errorf("save pool: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[+] Removed '%s' from pool.\n", args[0])
			return nil
		},
	}
}

func newPlexusListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List pool members",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projects := newPoolManager().List()
			out := cmd.OutOrStdout()
			if globals.JSON {
				if projects == nil {
					projects = []dag.ProjectInfo{}
				}
				return writeJSON(out, projects)
			}
			fmt.Fprintf(out, "[+] Registered Pool Members (%d):\n", len(projects))
			if len(projects) == 0 {
				fmt.Fprintln(out, "    (No projects registered. Use 'dendrite plexus add <dir>' to add members)")
				return nil
			}
			for i, p := range projects {
				fmt.Fprintf(out, "  %2d. %-25s [%s] -> %s\n", i+1, p.Name, joinEcosystems(p.Ecosystems), p.Path)
				if len(p.Manifests) > 0 {
					fmt.Fprintf(out, "      Manifests: %s\n", strings.Join(p.Manifests, ", "))
				}
			}
			return nil
		},
	}
}

func newPlexusResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve",
		Short: "Compute the topological build order and detect dependency cycles",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlexusResolve(newPoolManager().List(), cmd.OutOrStdout())
		},
	}
}

func runPlexusResolve(projects []dag.ProjectInfo, out io.Writer) error {
	if len(projects) == 0 {
		return fmt.Errorf("pool is empty: register directories first with 'dendrite plexus add <dir>'")
	}
	res, err := dag.NewDAGSolver().Resolve(projects)
	if err != nil {
		return fmt.Errorf("dependency DAG resolution: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, res)
	}
	fmt.Fprintln(out, "[+] Autonomous Multi-Directory DAG Resolution:")
	fmt.Fprintf(out, "    Nodes: %d | Edges: %d\n", len(res.Nodes), len(res.Edges))
	fmt.Fprintln(out, "    Topological Build Order (Foundational First):")
	for i, id := range res.TopologicalOrder {
		fmt.Fprintf(out, "      %2d. %-25s (depends on %d upstream)\n", i+1, id, len(res.Nodes[id].DependsOn))
	}
	if len(res.Edges) > 0 {
		fmt.Fprintln(out, "\n    Discovered Dependency Edges:")
		for _, edge := range res.Edges {
			fmt.Fprintf(out, "      * %s -> %s (via %s)\n", edge.From, edge.To, edge.Ecosystem)
		}
	}
	if len(res.LeafNodes) > 0 {
		fmt.Fprintf(out, "\n    Leaf Nodes (Foundational / Zero Dependencies): %s\n", strings.Join(res.LeafNodes, ", "))
	}
	if len(res.RootNodes) > 0 {
		fmt.Fprintf(out, "    Root Nodes (Top-Level / No Dependents):        %s\n", strings.Join(res.RootNodes, ", "))
	}
	return nil
}

func newPlexusSwitchCmd() *cobra.Command {
	req := switcher.SwitchRequest{TargetProjectDir: ".", SyncIDE: true}
	var mode string
	var noSyncIDE bool
	cmd := &cobra.Command{
		Use:   "switch <dependency>",
		Short: "Switch how a project consumes a dependency: published, branch, local or fork",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.DependencyName = args[0]
			req.Mode = switcher.DependencyMode(mode)
			req.SyncIDE = !noSyncIDE
			return runPlexusSwitch(cmd.Context(), req, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.StringVar(&req.TargetProjectDir, "in", ".", "Project whose manifest is rewritten")
	f.StringVar(&mode, "mode", "", "Dependency mode: published, branch, local or fork")
	f.StringVar(&req.Version, "version", "", "Published version (mode published)")
	f.StringVar(&req.BranchName, "branch", "", "Git branch (mode branch)")
	f.StringVar(&req.CommitSHA, "commit", "", "Git commit (mode branch)")
	f.StringVar(&req.LocalPath, "local-path", "", "Local checkout (mode local)")
	f.StringVar(&req.ForkURL, "fork-url", "", "Fork repository URL (mode fork)")
	f.StringVar(&req.ForkBranch, "fork-branch", "", "Fork branch (mode fork)")
	f.BoolVar(&noSyncIDE, "no-sync-ide", false, "Do not update the JetBrains scopes")
	requireFlags(cmd, "mode")
	return cmd
}

func runPlexusSwitch(ctx context.Context, req switcher.SwitchRequest, out io.Writer) error {
	result, err := switcher.NewDependencySwitcher().Switch(ctx, req)
	if err != nil {
		return fmt.Errorf("dependency mode switch: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "[+] Successfully switched '%s' dependency mode:\n", result.DependencyName)
	fmt.Fprintf(out, "    Target Project:     %s\n", result.TargetProject)
	fmt.Fprintf(out, "    Transition:         %s -> %s\n", result.PreviousMode, result.NewMode)
	fmt.Fprintf(out, "    Modified Files (%d): %s\n", len(result.ModifiedFiles), strings.Join(result.ModifiedFiles, ", "))
	if result.IDEScopesUpdated {
		fmt.Fprintln(out, "    JetBrains Scopes:   Synchronized (.idea/scopes/Dendrite_Local_Pool.xml)")
	}
	return nil
}

func newPlexusSplitCmd() *cobra.Command {
	req := split.SplitRequest{SourceProjectDir: "."}
	var noWire, noRegister bool
	cmd := &cobra.Command{
		Use:   "split",
		Short: "Extract a module into a new pool member (asks for the destination when --target-dir is empty)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			req.WireLocalMode = !noWire
			req.RegisterInPool = !noRegister
			in, out := cmd.InOrStdin(), cmd.OutOrStdout()
			req.Prompt = func(m *modal.DirectoryPickerModal) (string, error) { return m.RenderCLI(in, out) }
			return runPlexusSplit(cmd.Context(), req, out)
		},
	}
	f := cmd.Flags()
	f.StringVar(&req.SourceProjectDir, "from", ".", "Project to split")
	f.StringVar(&req.ModuleSubpath, "module", "", "Module path inside the project")
	f.StringVar(&req.NewMemberName, "name", "", "Name of the new pool member")
	f.StringVar(&req.TargetDirectory, "target-dir", "", "Destination directory")
	f.BoolVar(&noWire, "no-wire-local", false, "Do not switch the parent to the local dependency")
	f.BoolVar(&noRegister, "no-register-pool", false, "Do not add the new member to the pool")
	requireFlags(cmd, "module", "name")
	return cmd
}

func runPlexusSplit(ctx context.Context, req split.SplitRequest, out io.Writer) error {
	result, err := split.NewProjectSplitter().Split(ctx, req)
	if err != nil {
		return fmt.Errorf("project split: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "[+] Successfully split module into new pool member '%s':\n", result.NewMemberName)
	fmt.Fprintf(out, "    Destination:      %s\n", result.TargetDirectory)
	fmt.Fprintf(out, "    Manifest Created: %s\n", result.ManifestCreated)
	fmt.Fprintf(out, "    Extracted Files:  %d files moved\n", len(result.ExtractedFiles))
	if result.ParentWiredLocal {
		fmt.Fprintln(out, "    Parent Wired:     Local dependency mode active in parent project")
	}
	if result.PoolRegistered {
		fmt.Fprintln(out, "    Pool Registered:  Member added to .dendrite/pool.json")
	}
	return nil
}
