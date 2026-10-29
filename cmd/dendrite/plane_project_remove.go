package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"Dendrite/pkg/planesvc"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

type planeProjectRemoveOptions struct {
	Refs        []string
	Purge       bool
	KeepLocal   bool
	Force       bool
	Yes         bool
	Interactive bool // stdin is a terminal, so a confirmation can be answered
}

// removeTarget is one Plane project to remove and the local checkout bound to it, if known.
type removeTarget struct {
	Project plane.PlaneProject
	Dir     string // local checkout, "" when none is known
	EntryID string // its project ID in the global index, "" when it has none
}

func newChronosProjectRemoveCmd() *cobra.Command {
	var o planeProjectRemoveOptions
	cmd := &cobra.Command{
		Use:   "remove <IDENT|name|dir>...",
		Short: "Archive (default) or permanently delete Plane projects, keeping the local directories",
		Long: "Resolves each argument to a Plane project (identifier, name, UUID, or a directory linked to one) and\n" +
			"archives it; --purge deletes it for good with all its issues. The repository directories are never\n" +
			"touched except for the local Plane binding: the untracked .dendrite/plane.json is removed and the\n" +
			"project leaves the global index, and it is remembered in ~/.dendrite/untracked.json so\n" +
			"`chronos project audit` does not report it. Projects that are members of a synapse are refused\n" +
			"unless --force. Without --yes (-y) you confirm (--purge: by typing each project's name).",
		Args:    minArgs(1),
		Example: "dendrite chronos project remove OLDPROJ\ndendrite chronos project remove ~/work/old-repo --purge --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Refs = args
			o.Yes = globals.Yes
			o.Interactive = stdinIsTerminal()
			return planeRun(func(c *plane.Client) error {
				return planeProjectRemove(cmd.Context(), c, planesvc.Default(), o, cmd.InOrStdin(), cmd.OutOrStdout())
			})
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.Purge, "purge", false, "Delete permanently instead of archiving")
	f.BoolVar(&o.KeepLocal, "keep-local", false, "Leave the local Plane binding and index entry alone")
	f.BoolVar(&o.Force, "force", false, "Also remove projects that are members of a synapse")
	return cmd
}

func planeProjectRemove(ctx context.Context, client *plane.Client, svc *planesvc.Service, o planeProjectRemoveOptions, in io.Reader, out io.Writer) error {
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		return err
	}
	ix, err := projectid.LoadIndex(indexPath)
	if err != nil {
		return err
	}
	targets, err := resolveRemoveTargets(ctx, client, o.Refs, ix)
	if err != nil {
		return err
	}
	if err := refuseSynapseMembers(targets, o.Force); err != nil {
		return err
	}

	verb := "ARCHIVE"
	if o.Purge {
		verb = "DELETE PERMANENTLY"
	}
	fmt.Fprintf(out, "[i] Will %s %d Plane project(s):\n", verb, len(targets))
	for _, t := range targets {
		local := "no local checkout known"
		if t.Dir != "" {
			local = t.Dir
		}
		fmt.Fprintf(out, "    %-10s %-36s %s\n", t.Project.Identifier, t.Project.Name, local)
	}

	if !o.Yes {
		if !o.Interactive {
			return errors.New("refusing to remove Plane projects without a terminal to confirm on: pass --yes")
		}
		reader := bufio.NewReader(in)
		if o.Purge {
			for _, t := range targets {
				fmt.Fprintf(out, "[?] %s and every issue in it will be lost. Type the project name (%s) to confirm: ", t.Project.Identifier, t.Project.Name)
				line, _ := reader.ReadString('\n')
				if strings.TrimSpace(line) != t.Project.Name {
					return fmt.Errorf("aborted: %q does not match %q, nothing was removed", strings.TrimSpace(line), t.Project.Name)
				}
			}
		} else {
			fmt.Fprint(out, "[?] Continue? [y/N] ")
			line, _ := reader.ReadString('\n')
			if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
				return errors.New("aborted: nothing was removed")
			}
		}
	}

	untracked, err := loadUntracked()
	if err != nil {
		return err
	}
	var failures []error
	removed := 0
	for _, t := range targets {
		var callErr error
		if o.Purge {
			callErr = svc.DeleteProject(ctx, client, t.Project.ID)
		} else {
			callErr = svc.ArchiveProject(ctx, client, t.Project.ID)
		}
		if callErr != nil {
			failures = append(failures, fmt.Errorf("%s: %w", t.Project.Identifier, callErr))
			fmt.Fprintf(out, "[-] %s: %v\n", t.Project.Identifier, callErr)
			continue
		}
		removed++
		done := map[bool]string{true: "deleted", false: "archived"}[o.Purge]
		fmt.Fprintf(out, "[✓] %s (%s) %s in Plane\n", t.Project.Identifier, t.Project.Name, done)
		if !o.KeepLocal {
			detachLocal(t, ix, untracked, out)
		}
	}
	if removed > 0 && !o.KeepLocal {
		if err := ix.Save(); err != nil {
			failures = append(failures, err)
		}
		if err := untracked.Save(); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// resolveRemoveTargets maps each argument to a Plane project: a directory linked to one, or an
// identifier, name or UUID. Duplicates collapse; an unknown or ambiguous name is an error.
func resolveRemoveTargets(ctx context.Context, client *plane.Client, refs []string, ix *projectid.Index) ([]removeTarget, error) {
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	var targets []removeTarget
	seen := map[string]bool{}
	for _, ref := range refs {
		var proj *plane.PlaneProject
		dir := ""
		if info, statErr := os.Stat(ref); statErr == nil && info.IsDir() {
			link, root, linkErr := plane.LinkForDir(ref)
			if linkErr != nil {
				return nil, fmt.Errorf("%s is not linked to a Plane project: %w", ref, linkErr)
			}
			dir = root
			for i := range projects {
				if projects[i].ID == link.ProjectID {
					proj = &projects[i]
				}
			}
			if proj == nil {
				return nil, fmt.Errorf("%s is linked to Plane project %s, which no longer exists", ref, link.ProjectID)
			}
		} else {
			var matches []*plane.PlaneProject
			for i := range projects {
				p := &projects[i]
				if p.ID == ref || strings.EqualFold(p.Identifier, ref) || strings.EqualFold(p.Name, ref) {
					matches = append(matches, p)
				}
			}
			switch len(matches) {
			case 0:
				return nil, fmt.Errorf("no Plane project matches %q", ref)
			case 1:
				proj = matches[0]
			default:
				var ids []string
				for _, m := range matches {
					ids = append(ids, m.Identifier)
				}
				return nil, fmt.Errorf("%q is ambiguous: it matches %s; use the identifier", ref, strings.Join(ids, ", "))
			}
		}
		if seen[proj.ID] {
			continue
		}
		seen[proj.ID] = true
		t := removeTarget{Project: *proj, Dir: dir}
		t.Dir, t.EntryID = localCheckout(ix, proj.ID, dir)
		targets = append(targets, t)
	}
	return targets, nil
}

// localCheckout finds the repository bound to a Plane project: dir when the caller gave one,
// otherwise an index entry whose recorded binding or plane.json names the project.
func localCheckout(ix *projectid.Index, planeProjectID, dir string) (string, string) {
	if dir != "" {
		if e, ok := ix.LookupByPath(dir); ok {
			return dir, e.ID
		}
		id, _ := projectid.Read(dir)
		return dir, id
	}
	for _, e := range ix.List() {
		if e.Plane != nil && e.Plane.ProjectID == planeProjectID {
			return e.Path, e.ID
		}
		if l, err := plane.ReadLink(e.Path); err == nil && l.ProjectID == planeProjectID {
			return e.Path, e.ID
		}
	}
	return "", ""
}

// refuseSynapseMembers stops the removal of a project that a synapse still lists, since its peers
// would lose their board.
func refuseSynapseMembers(targets []removeTarget, force bool) error {
	if force {
		return nil
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		return nil
	}
	syns, err := st.List()
	if err != nil {
		return nil
	}
	var in []string
	for _, t := range targets {
		for _, s := range syns {
			for _, m := range s.Members {
				if (t.EntryID != "" && m.ProjectID == t.EntryID) || (m.PlaneProject != "" && strings.EqualFold(m.PlaneProject, t.Project.Identifier)) {
					in = append(in, fmt.Sprintf("%s is a member of synapse %s", t.Project.Identifier, s.ID))
				}
			}
		}
	}
	if len(in) > 0 {
		return fmt.Errorf("%s; remove it from the synapse first (or pass --force)", strings.Join(in, "; "))
	}
	return nil
}

// detachLocal removes the local Plane binding of a removed project: the untracked plane.json goes,
// the project leaves the index, and the audit is told not to expect a Plane project there. The
// directory and its project.id stay.
func detachLocal(t removeTarget, ix *projectid.Index, untracked *projectid.Untracked, out io.Writer) {
	if t.Dir == "" {
		fmt.Fprintf(out, "    [i] no local checkout is known for %s; nothing to detach\n", t.Project.Identifier)
		return
	}
	if err := os.Remove(plane.LinkPath(t.Dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(out, "    [-] could not remove %s: %v\n", plane.LinkPath(t.Dir), err)
	}
	if t.EntryID != "" {
		ix.Remove(t.EntryID)
		untracked.Add(t.EntryID, filepath.Base(t.Dir), t.Dir)
	}
	fmt.Fprintf(out, "    [i] detached %s (the directory stays)\n", t.Dir)
}

func loadUntracked() (*projectid.Untracked, error) {
	path, err := projectid.DefaultUntrackedPath()
	if err != nil {
		return nil, err
	}
	return projectid.LoadUntracked(path)
}
