package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/pkg/env/bootstrap"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/tracker/plane"
)

// stateIgnoreLine is what keeps a repository's .dendrite/ out of git.
const stateIgnoreLine = ".dendrite/"

type migrateStateReport struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Skipped     string `json:"skipped,omitempty"`
	Untracked   int    `json:"untracked_files"`
	Gitignore   bool   `json:"gitignore_updated"`
	RootCommit  string `json:"root_commit,omitempty"`
	PlaneRecord bool   `json:"plane_recorded"`
}

func newSomaMigrateStateCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "migrate-state",
		Short: "Stop tracking .dendrite/ in every indexed repository (nothing is committed)",
		Long: ".dendrite/ is local tool state. For each repository in the global project index this records\n" +
			"its root commit and Plane binding in the index (so a fresh clone recovers both), adds\n" +
			".dendrite/ to .gitignore and runs `git rm -r --cached .dendrite`. The files stay on disk and\n" +
			"the changes are left uncommitted for you to review and commit per repository.",
		Args:    noArgs,
		Example: "dendrite soma migrate-state --dry-run\ndendrite soma migrate-state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSomaMigrateState(dryRun, cmd.OutOrStdout())
		},
	}
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func runSomaMigrateState(dryRun bool, out io.Writer) error {
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		return err
	}
	ix, err := projectid.LoadIndex(indexPath)
	if err != nil {
		return err
	}

	var reports []migrateStateReport
	for _, e := range ix.List() {
		reports = append(reports, migrateRepoState(ix, e, dryRun))
	}
	if !dryRun {
		if err := ix.Save(); err != nil {
			return err
		}
	}
	if globals.JSON {
		return writeJSON(out, reports)
	}
	for _, r := range reports {
		switch {
		case r.Skipped != "":
			fmt.Fprintf(out, "[-] %-32s skipped: %s\n", r.Name, r.Skipped)
		default:
			fmt.Fprintf(out, "[✓] %-32s untracked %d file(s), .gitignore %s, plane %s\n", r.Name, r.Untracked,
				map[bool]string{true: "updated", false: "ok"}[r.Gitignore],
				map[bool]string{true: "recorded", false: "none"}[r.PlaneRecord])
		}
	}
	if dryRun {
		fmt.Fprintln(out, "[i] dry run: nothing was changed")
	} else {
		fmt.Fprintln(out, "[i] changes are uncommitted; review each repository with `git status` and commit what you want")
	}
	return nil
}

// migrateRepoState untracks .dendrite/ in one repository and records its identity in the index.
func migrateRepoState(ix *projectid.Index, e projectid.Entry, dryRun bool) migrateStateReport {
	r := migrateStateReport{Path: e.Path, Name: e.Name}
	if info, err := os.Stat(e.Path); err != nil || !info.IsDir() {
		r.Skipped = "path is missing (run `dendrite chronos project refresh`)"
		return r
	}
	if root := projectid.StateRoot(e.Path); root != e.Path {
		r.Skipped = "is a linked worktree of " + root
		return r
	}
	if id, err := projectid.Read(e.Path); err != nil || id != e.ID {
		r.Skipped = "its .dendrite/project.id does not match the index"
		return r
	}
	if _, err := os.Stat(e.Path + "/.git"); err != nil {
		r.Skipped = "not a git repository"
		return r
	}

	r.RootCommit = projectid.GitRootCommit(e.Path)
	link, linkErr := plane.ReadLink(e.Path)
	r.PlaneRecord = linkErr == nil
	if !dryRun {
		updated := e
		updated.RootCommit = r.RootCommit
		if linkErr == nil {
			updated.Plane = &projectid.PlaneBinding{Identifier: link.Identifier, ProjectID: link.ProjectID, Name: link.Name, Workspace: link.Workspace}
		}
		ix.Upsert(updated)
	}

	changed, err := bootstrap.EnsureGitignoreLine(e.Path, stateIgnoreLine, dryRun)
	if err != nil {
		r.Skipped = "cannot update .gitignore: " + err.Error()
		return r
	}
	r.Gitignore = changed

	tracked, _ := exec.Command("git", "-C", e.Path, "ls-files", "--", ".dendrite").Output()
	r.Untracked = len(strings.Fields(string(tracked)))
	if r.Untracked > 0 && !dryRun {
		if out, err := exec.Command("git", "-C", e.Path, "rm", "-r", "--cached", "-q", "--ignore-unmatch", "--", ".dendrite").CombinedOutput(); err != nil {
			r.Skipped = "git rm --cached failed: " + strings.TrimSpace(string(out))
			r.Untracked = 0
		}
	}
	return r
}
