package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"Dendrite/pkg/distcodebase"
	"Dendrite/pkg/tracker/plane"

	"github.com/spf13/cobra"
)

// newMeshCmd builds `dendrite mesh` (alias dist): the distributed codebase registry, decomposer and spawner.
func newMeshCmd() *cobra.Command {
	cmd := newGroupCmd("mesh", "Distributed codebase spawner, modular REA, and repo coordinator", "axon",
		"dist", "distcodebase", "distributed", "neural-mesh")
	cmd.Example = "dendrite mesh sources\n" +
		"dendrite mesh resolve totp go\n" +
		"dendrite mesh decompose my-emulator ./spec.md\n" +
		"dendrite mesh spawn auth-svc ../auth-svc go --satellite --no-plane"

	reg := distcodebase.NewDefaultRegistry()
	cmd.AddCommand(
		&cobra.Command{
			Use:     "sources",
			Aliases: []string{"list-sources"},
			Short:   "List codebase sources and registered libraries",
			Args:    noArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runMeshSources(reg, cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:   "resolve <capability> [language]",
			Short: "Find the highest-priority library that provides a capability",
			Args:  rangeArgs(1, 2),
			RunE: func(cmd *cobra.Command, args []string) error {
				lang := ""
				if len(args) > 1 {
					lang = args[1]
				}
				return runMeshResolve(reg, args[0], lang, cmd.OutOrStdout())
			},
		},
		&cobra.Command{
			Use:   "decompose <project> [target-spec]",
			Short: "Plan a forced modular decomposition into reused libraries, custom modules and satellites",
			Args:  rangeArgs(1, 2),
			RunE: func(cmd *cobra.Command, args []string) error {
				target := ""
				if len(args) > 1 {
					target = args[1]
				}
				return runMeshDecompose(reg, args[0], target, cmd.OutOrStdout())
			},
		},
		newMeshInteropCmd(reg),
		newMeshSpawnCmd(),
	)
	return cmd
}

func runMeshSources(reg *distcodebase.Registry, out io.Writer) error {
	sources, libs := reg.ListSources(), reg.ListLibraries()
	if globals.JSON {
		return writeJSON(out, map[string]any{"sources": sources, "libraries": libs})
	}
	fmt.Fprintf(out, "[+] Distributed Codebase Sources (%d):\n", len(sources))
	for _, s := range sources {
		fmt.Fprintf(out, "    [%s] Priority %d | %-25s | URL: %s\n", s.Type, s.Priority, s.Name, s.URL)
	}
	fmt.Fprintf(out, "\n[+] Registered Ecosystem Libraries & Tools (%d):\n", len(libs))
	for _, l := range libs {
		ownedTag := ""
		if l.IsOwned {
			ownedTag = " (USER OWNED - Priority 1)"
		}
		fmt.Fprintf(out, "    - %-15s (Lang: %-4s, P%d)%s: %s\n", l.Name, l.Language, l.Priority, ownedTag, l.Description)
		fmt.Fprintf(out, "      Capabilities: %s\n", strings.Join(l.Capabilities, ", "))
	}
	return nil
}

func runMeshResolve(reg *distcodebase.Registry, capability, lang string, out io.Writer) error {
	lib, found := reg.ResolveCapability(capability, lang)
	if !found {
		return fmt.Errorf("no library provides capability '%s'", capability)
	}
	if globals.JSON {
		return writeJSON(out, lib)
	}
	fmt.Fprintf(out, "[+] Highest Priority Match for '%s':\n", capability)
	fmt.Fprintf(out, "    Library:     %s (Priority %d, Owned: %t)\n", lib.Name, lib.Priority, lib.IsOwned)
	fmt.Fprintf(out, "    Package:     %s\n", lib.RegistryPackage)
	fmt.Fprintf(out, "    Repository:  %s\n", lib.RepositoryURL)
	fmt.Fprintf(out, "    Capabilities: %s\n", strings.Join(lib.Capabilities, ", "))
	return nil
}

func runMeshDecompose(reg *distcodebase.Registry, project, target string, out io.Writer) error {
	forced := true
	plan, err := distcodebase.NewDecomposer(reg).DecomposeProject(distcodebase.DecomposeRequest{
		ProjectName:        project,
		TargetBinaryOrSpec: target,
		Language:           "rust",
		ForcedModular:      &forced,
	})
	if err != nil {
		return fmt.Errorf("decomposition: %w", err)
	}
	if globals.JSON {
		return writeJSON(out, plan)
	}
	fmt.Fprintf(out, "[+] Forced Modular Decomposition Plan for '%s':\n", project)
	fmt.Fprintf(out, "    Total Modules:     %d\n", plan.TotalModules)
	fmt.Fprintf(out, "    Reused Libraries:  %d\n", plan.ReusedLibrariesCount)
	fmt.Fprintf(out, "    Custom Dev:        %d\n", plan.CustomModulesCount)
	fmt.Fprintf(out, "    Satellite Spawns:  %d\n\n", plan.SatelliteProjectsCount)
	for _, m := range plan.Modules {
		switch {
		case m.Strategy == distcodebase.StrategyReuseLibrary && m.MatchedLibrary != nil:
			fmt.Fprintf(out, "    - Module '%s': REUSE -> %s (Priority %d, Repo: %s)\n",
				m.Name, m.MatchedLibrary.Name, m.MatchedLibrary.Priority, m.MatchedLibrary.RepositoryURL)
		case m.Strategy == distcodebase.StrategyDevelopCustom:
			fmt.Fprintf(out, "    - Module '%s': CUSTOM DEV -> Reason: %s\n", m.Name, m.DevelopCustomReason)
		default:
			fmt.Fprintf(out, "    - Module '%s': SATELLITE PROJECT -> Spawn as standalone project\n", m.Name)
		}
	}
	return nil
}

// newMeshInteropCmd builds the cross-project feature coordination subtree.
// Coordinations live in memory, so complete and reconcile only see ones made in the same process.
func newMeshInteropCmd(reg *distcodebase.Registry) *cobra.Command {
	cmd := newGroupCmd("interop", "Coordinate a feature an owned dependency is missing", "")
	coord := distcodebase.NewInteropCoordinator(reg)
	cmd.AddCommand(
		&cobra.Command{
			Use:   "coordinate <requester> <dependency> <feature>",
			Short: "Open a coordination and emit an unblocking fallback marker",
			Args:  exactArgs(3),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := coord.CoordinateFeature(distcodebase.InteropRequest{
					RequesterProject:  args[0],
					DependencyProject: args[1],
					FeatureRequested:  args[2],
					AllowFallback:     true,
				})
				if err != nil {
					return fmt.Errorf("interop coordination: %w", err)
				}
				out := cmd.OutOrStdout()
				if globals.JSON {
					return writeJSON(out, c)
				}
				fmt.Fprintf(out, "[+] Interop Coordination Initialized (%s):\n", c.ID)
				fmt.Fprintf(out, "    Requester:  %s\n", c.RequesterProject)
				fmt.Fprintf(out, "    Dependency: %s (User Owned: %t)\n", c.DependencyProject, c.DependencyIsOwned)
				fmt.Fprintf(out, "    Feature:    %s\n", c.FeatureRequested)
				fmt.Fprintf(out, "    Status:     %s\n", c.State)
				fmt.Fprintf(out, "    Marker:     %s\n", c.FallbackTODO)
				return nil
			},
		},
		&cobra.Command{
			Use:   "complete <coordination-id>",
			Short: "Mark the dependency's feature as delivered",
			Args:  exactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := coord.CompleteDependencyFeature(args[0])
				if err != nil {
					return fmt.Errorf("complete feature: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[+] Dependency feature marked completed for %s. Ready to reconcile.\n", c.ID)
				return nil
			},
		},
		&cobra.Command{
			Use:   "reconcile <coordination-id>",
			Short: "Replace the fallback in the requester with the delivered feature",
			Args:  exactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				rec, err := coord.ReconcileProject(args[0])
				if err != nil {
					return fmt.Errorf("reconcile project: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[+] Interop Reconciled Successfully (%s):\n", rec.CoordinationID)
				fmt.Fprintf(cmd.OutOrStdout(), "    Patch applied to %s:\n%s\n", rec.Requester, rec.PatchGenerated)
				return nil
			},
		},
		&cobra.Command{
			Use:     "list",
			Aliases: []string{"ls"},
			Short:   "List coordinations",
			Args:    noArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				list := coord.ListCoordinations()
				out := cmd.OutOrStdout()
				if globals.JSON {
					return writeJSON(out, list)
				}
				fmt.Fprintf(out, "[+] Active Interop Coordinations (%d):\n", len(list))
				for _, c := range list {
					fmt.Fprintf(out, "    - %s: %s -> %s [%s] (Feature: %s)\n", c.ID, c.RequesterProject, c.DependencyProject, c.State, c.FeatureRequested)
				}
				return nil
			},
		},
	)
	return cmd
}

type meshSpawnOptions struct {
	Satellite   bool
	NoPlane     bool
	Identifier  string
	Description string
}

func newMeshSpawnCmd() *cobra.Command {
	var o meshSpawnOptions
	cmd := &cobra.Command{
		Use:   "spawn <project> <target-dir> [language]",
		Short: "Create a parallel or satellite project (git repo, workspace entry and Plane project)",
		Args:  rangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			lang := "rust"
			if len(args) > 2 {
				lang = args[2]
			}
			if o.Description == "" {
				o.Description = fmt.Sprintf("Autonomous project %s generated via Dendrite", args[0])
			}
			return runMeshSpawn(cmd.Context(), args[0], args[1], lang, o, cmd.OutOrStdout())
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.Satellite, "satellite", false, "Spawn as a satellite of the current project")
	f.BoolVar(&o.NoPlane, "no-plane", false, "Do not create a Plane project")
	f.StringVar(&o.Identifier, "identifier", "", "Plane project identifier (e.g. AUTH)")
	f.StringVar(&o.Description, "desc", "", "Project description")
	return cmd
}

func runMeshSpawn(ctx context.Context, project, dir, lang string, o meshSpawnOptions, out io.Writer) error {
	var planeClient *plane.Client
	if !o.NoPlane {
		planeClient, _ = newPlaneClient()
	}
	wsReg, _ := newProjectRegistry()
	res, err := distcodebase.NewProjectSpawner(planeClient, wsReg).Spawn(ctx, distcodebase.SpawnRequest{
		ProjectName:        project,
		TargetDir:          dir,
		Language:           lang,
		IsSatellite:        o.Satellite,
		CreatePlaneProject: planeClient != nil,
		InitGit:            true,
		Identifier:         strings.ToUpper(o.Identifier),
		Description:        o.Description,
	})
	if err != nil {
		return fmt.Errorf("spawn project: %w", err)
	}
	fmt.Fprintf(out, "[+] Spawned Project '%s' Successfully:\n", res.ProjectName)
	fmt.Fprintf(out, "    Path:            %s\n", res.ProjectPath)
	fmt.Fprintf(out, "    Git Initialized: %t\n", res.GitInitialized)
	if res.PlaneProjectID != "" {
		fmt.Fprintf(out, "    Plane Project:   %s\n", res.PlaneProjectID)
	}
	if res.PlaneError != "" {
		return fmt.Errorf("Plane project was not created: %s", res.PlaneError)
	}
	return nil
}
