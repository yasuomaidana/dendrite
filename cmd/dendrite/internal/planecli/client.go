package planecli

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/ticketlock"
	"Dendrite/pkg/tracker/plane"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// lifecycleIn builds the ticket lifecycle for the working copy in worktree ("" is the current directory).
func LifecycleIn(c *plane.Client, worktree string) *ticketlock.Lifecycle {
	if worktree == "" {
		worktree = "."
	}
	if abs, err := filepath.Abs(worktree); err == nil {
		worktree = abs
	}
	return ticketlock.NewLifecycle(ticketlock.Board{Svc: planesvc.Default(), Client: c}, worktree)
}

// newPlaneClient is a seam so tests can stub the Plane client.
var NewPlaneClient = plane.NewClientFromEnv

// planeRun opens the Plane client and runs fn with it.
func PlaneRun(fn func(*plane.Client) error) error {
	client, err := NewPlaneClient()
	if err != nil {
		return fmt.Errorf("initialize Plane client: %w", err)
	}
	return fn(client)
}

// runPlaneDocSync synchronizes a docs directory with Plane pages; `codex plane` calls it.
func RunPlaneDocSync(ctx context.Context, direction, dir string, out io.Writer) error {
	var d plane.SyncDirection
	switch direction {
	case "push", "to-plane":
		d = plane.DirectionToPlane
	case "pull", "from-plane":
		d = plane.DirectionFromPlane
	case "sync", "both", "bidirectional":
		d = plane.DirectionBidirectional
	default:
		return cli.UsageErrorf("unknown sync direction %q: use push, pull or sync", direction)
	}
	return PlaneRun(func(client *plane.Client) error {
		fmt.Fprintf(out, "[+] Synchronizing project documentation (%s) with Plane (%s)...\n", d, dir)
		report, err := plane.SyncDocs(ctx, client, dir, d)
		if report != nil {
			fmt.Fprintf(out, "[✓] Documentation sync completed in %v\n", report.Duration)
			fmt.Fprintf(out, "    - Files Pushed to Plane:   %d\n", report.FilesPushed)
			fmt.Fprintf(out, "    - Files Pulled from Plane: %d\n", report.FilesPulled)
			fmt.Fprintf(out, "    - Pages Created:           %d\n", report.PagesCreated)
			fmt.Fprintf(out, "    - Pages Updated:           %d\n", report.PagesUpdated)
			if len(report.Errors) > 0 {
				fmt.Fprintf(out, "    - Warnings/Errors (%d):\n", len(report.Errors))
				for _, e := range report.Errors {
					fmt.Fprintf(out, "        * %s\n", e)
				}
			}
		}
		if err != nil {
			return fmt.Errorf("documentation sync: %w", err)
		}
		return nil
	})
}

// warnStaleIndex prints one line when entries of the global project index point at directories
// that no longer exist. It is a cheap stat per entry and never fails the caller.
func WarnStaleIndex(out io.Writer) {
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		return
	}
	ix, err := projectid.LoadIndex(indexPath)
	if err != nil {
		return
	}
	stale := 0
	for _, e := range ix.List() {
		if _, err := os.Stat(e.Path); err != nil {
			stale++
		}
	}
	if stale > 0 {
		fmt.Fprintf(out, "[!] %d project(s) in the global index point at paths that no longer exist; run `dendrite chronos project refresh --dry-run`\n", stale)
	}
}
