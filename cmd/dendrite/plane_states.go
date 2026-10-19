package main

import (
	"context"
	"fmt"
	"strings"

	"Dendrite/pkg/tracker/plane"
)

const planeStatesUsage = "Usage: dendrite plane states reconcile|ensure [--project <uuid|IDENT>] [--all] [--dry-run]"

// handlePlaneStates converges one project (or every workspace project with --all)
// onto plane.CanonicalStates. "ensure" is kept as an alias of "reconcile".
func handlePlaneStates(ctx context.Context, client *plane.Client, args []string) {
	if len(args) == 0 || (args[0] != "ensure" && args[0] != "reconcile") {
		fmt.Println(planeStatesUsage)
		return
	}
	projectRef := ""
	all, dryRun := false, false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--project", "-p":
			if i+1 >= len(args) {
				fmt.Println(planeStatesUsage)
				return
			}
			projectRef = args[i+1]
			i++
		case "--all":
			all = true
		case "--dry-run":
			dryRun = true
		default:
			fmt.Println(planeStatesUsage)
			return
		}
	}

	type target struct{ id, label string }
	var targets []target
	switch {
	case all:
		projects, err := client.ListProjects(ctx)
		if err != nil {
			fmt.Printf("[-] Failed to list Plane projects: %v\n", err)
			return
		}
		for _, p := range projects {
			targets = append(targets, target{p.ID, p.Identifier})
		}
	case projectRef != "":
		// Accept identifiers like DGIT; fall back to treating the value as a raw UUID.
		if p, err := client.ResolveProject(ctx, projectRef); err == nil {
			targets = append(targets, target{p.ID, p.Identifier})
		} else {
			targets = append(targets, target{projectRef, projectRef})
		}
	default:
		targets = append(targets, target{client.ProjectID(), client.ProjectID()})
	}

	for _, t := range targets {
		rep, err := client.ReconcileStates(ctx, t.id, dryRun)
		if err != nil {
			fmt.Printf("[-] Failed to reconcile states for %s: %v\n", t.label, err)
			continue
		}
		verb := "Reconciled"
		if dryRun {
			verb = "Would reconcile"
		}
		if len(rep.Changes) == 0 {
			fmt.Printf("[✓] %s: states already canonical\n", t.label)
		} else {
			fmt.Printf("[✓] %s states for %s:\n", verb, t.label)
			for _, ch := range rep.Changes {
				switch {
				case ch.Action == "create":
					fmt.Printf("    + create %s\n", ch.Name)
				case ch.From != "":
					fmt.Printf("    ~ %s -> %s (%s)\n", ch.From, ch.Name, strings.Join(ch.Fields, ", "))
				default:
					fmt.Printf("    ~ %s (%s)\n", ch.Name, strings.Join(ch.Fields, ", "))
				}
			}
		}
		if len(rep.Unmanaged) > 0 {
			fmt.Printf("    ? unmanaged (left untouched): %s\n", strings.Join(rep.Unmanaged, ", "))
		}
	}
}
