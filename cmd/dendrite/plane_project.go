package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"Dendrite/pkg/projectaudit"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"

	"github.com/spf13/cobra"
)

// errAuditViolations signals that the audit found repositories breaking the project rule.
var errAuditViolations = errors.New("project rule violations found")

// planeProjectEnsureOptions are the `chronos project ensure` flags.
type planeProjectEnsureOptions struct {
	Dir         string
	Identifier  string
	Name        string
	Description string
	Import      string
}

// newChronosProjectCmd builds `chronos project`: the repository's Plane project link and the project rule audit.
func newChronosProjectCmd() *cobra.Command {
	cmd := newGroupCmd("project", "Link the repository to its Plane project and audit the project rule", "")
	var eo planeProjectEnsureOptions
	ensure := &cobra.Command{
		Use:   "ensure [dir]",
		Short: "Adopt or create the repository's Plane project, converge its states, and write .dendrite/plane.json",
		Long: "Adopt or create the repository's Plane project, converge its states, and write .dendrite/plane.json.\n" +
			"--import loads a markdown checklist (e.g. DEV_STATUS.md) as issues, one module per milestone.",
		Args:    maxArgs(1),
		Example: "dendrite chronos project ensure\ndendrite chronos project ensure ../dendrite-git-rs --identifier DGIT --import DEV_STATUS.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			eo.Dir = "."
			if len(args) == 1 {
				eo.Dir = args[0]
			}
			return planeRun(func(c *plane.Client) error {
				return planeProjectEnsure(cmd.Context(), c, eo, cmd.OutOrStdout())
			})
		},
	}
	f := ensure.Flags()
	f.StringVar(&eo.Identifier, "identifier", "", "Plane project identifier (default: the linked one, else derived from the name)")
	f.StringVar(&eo.Name, "name", "", "Plane project name (default: the repository directory name)")
	f.StringVar(&eo.Description, "description", "", "Plane project description")
	f.StringVar(&eo.Import, "import", "", "Markdown checklist to import as issues")

	show := &cobra.Command{
		Use:     "show [dir]",
		Short:   "Print the Plane project linked to the repository",
		Args:    maxArgs(1),
		Example: "dendrite chronos project show",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return planeProjectShow(dir, cmd.OutOrStdout())
		},
	}

	var roots, owners []string
	audit := &cobra.Command{
		Use:   "audit",
		Short: "Check that every editable repository is a Plane project and related repositories share a synapse",
		Long: "Check the rule: every editable repository is a Plane project, and repositories that\n" +
			"depend on each other share a synapse. Exits non-zero when a repository breaks it.",
		Args:    noArgs,
		Example: "dendrite chronos project audit\ndendrite chronos project audit --roots ~/GoLandProjects,~/RustRoverProjects --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return planeProjectAudit(roots, owners, cmd.OutOrStdout())
		},
	}
	audit.Flags().StringSliceVar(&roots, "roots", nil, "Directories to scan (default: parents of every known project and synapse member)")
	audit.Flags().StringSliceVar(&owners, "owner", nil, "Git remote owners whose repositories are editable (default: this repository's owner)")

	cmd.AddCommand(ensure, show, audit)
	return cmd
}

func planeProjectEnsure(ctx context.Context, client *plane.Client, o planeProjectEnsureOptions, out io.Writer) error {
	dir, identifier, name, description, importFile := o.Dir, o.Identifier, o.Name, o.Description, o.Import
	root, err := plane.FindRepoRoot(dir)
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	if name == "" {
		name = filepath.Base(root)
	}
	if identifier == "" {
		if link, err := plane.ReadLink(root); err == nil {
			identifier = link.Identifier
		} else {
			identifier = plane.SuggestIdentifier(name)
		}
	}
	if description == "" {
		description = fmt.Sprintf("Work tracked for the %s repository (%s).", name, root)
	}

	proj, created, err := client.EnsureProject(ctx, name, identifier, description)
	if err != nil {
		return err
	}
	link := plane.ProjectLink{Identifier: proj.Identifier, ProjectID: proj.ID, Name: proj.Name, Workspace: client.WorkspaceSlug()}
	if err := plane.WriteLink(root, link); err != nil {
		return err
	}
	if err := registerProjectIdentity(root); err != nil {
		return err
	}
	verb := "Adopted"
	if created {
		verb = "Created"
	}
	fmt.Fprintf(out, "[✓] %s Plane project %s (%s) for %s\n", verb, proj.Identifier, proj.Name, root)
	fmt.Fprintf(out, "    Link: %s\n", plane.LinkPath(root))

	if importFile == "" {
		return nil
	}
	path := importFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	md, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rel, _ := filepath.Rel(root, path)
	rep, err := plane.ImportChecklist(ctx, client.ForProject(proj.ID), plane.ParseChecklist(string(md)), rel)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "    Imported %s: %d created, %d already present, %d new modules\n", rel, rep.Created, rep.Existing, rep.Modules)
	return nil
}

// registerProjectIdentity makes sure the repository has a project ID and is in the global index.
func registerProjectIdentity(root string) error {
	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		return err
	}
	idx, err := projectid.LoadIndex(indexPath)
	if err != nil {
		return err
	}
	entry, err := idx.Register(root)
	if err != nil {
		return err
	}
	if l, err := plane.ReadLink(root); err == nil {
		idx.SetPlane(entry.ID, &projectid.PlaneBinding{Identifier: l.Identifier, ProjectID: l.ProjectID, Name: l.Name, Workspace: l.Workspace})
	}
	return idx.Save()
}

func planeProjectShow(dir string, out io.Writer) error {
	link, root, err := plane.LinkForDir(dir)
	if err != nil {
		if root != "" {
			return fmt.Errorf("%s has no Plane project; run `dendrite chronos project ensure %s`", root, root)
		}
		return err
	}
	fmt.Fprintf(out, "%s -> %s (%s) project_id=%s workspace=%s\n", root, link.Identifier, link.Name, link.ProjectID, link.Workspace)
	return nil
}

func planeProjectAudit(rawRoots, owners []string, out io.Writer) error {
	var roots []string
	for _, r := range rawRoots {
		roots = append(roots, expandHome(r))
	}
	var syns []*synapse.Synapse
	if st, err := synapse.DefaultStore(); err == nil {
		syns, _ = st.List()
	}
	if len(roots) == 0 {
		roots = defaultAuditRoots(syns)
	}
	if len(owners) == 0 {
		owners = projectaudit.GitHubOwners(func(args ...string) ([]byte, error) {
			return exec.Command("gh", args...).Output()
		})
	}
	if len(owners) == 0 {
		if cwdRoot, err := plane.FindRepoRoot("."); err == nil {
			if o := projectaudit.OwnerOf(projectid.NormalizeRemote(projectid.GitRemote(cwdRoot))); o != "" {
				owners = []string{o}
			}
		}
	}

	rep := projectaudit.Audit(projectaudit.Discover(roots), owners, syns)
	if globals.JSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return err
		}
	} else {
		printAudit(out, rep, roots, owners)
	}
	if len(rep.MissingProject) > 0 || len(rep.Unsynapsed) > 0 {
		return errAuditViolations
	}
	return nil
}

// expandHome resolves a leading "~/" (the shell only expands the first one in a comma list).
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// defaultAuditRoots returns the parent directories of every known project and synapse member.
func defaultAuditRoots(syns []*synapse.Synapse) []string {
	set := map[string]bool{}
	if indexPath, err := projectid.DefaultIndexPath(); err == nil {
		if idx, err := projectid.LoadIndex(indexPath); err == nil {
			for _, e := range idx.List() {
				set[filepath.Dir(e.Path)] = true
			}
		}
	}
	for _, s := range syns {
		for _, m := range s.Members {
			if dir, err := synapse.Resolve(m); err == nil {
				set[filepath.Dir(dir)] = true
			}
		}
	}
	if cwdRoot, err := plane.FindRepoRoot("."); err == nil {
		set[filepath.Dir(cwdRoot)] = true
	}
	var roots []string
	for r := range set {
		roots = append(roots, r)
	}
	sort.Strings(roots)
	return roots
}

func printAudit(out io.Writer, rep projectaudit.Report, roots, owners []string) {
	fmt.Fprintf(out, "[+] Project rule audit over %s (editable owners: %s)\n\n", strings.Join(roots, ", "), strings.Join(owners, ", "))
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "REPOSITORY\tEDITABLE\tPLANE\tSYNAPSES")
	for _, r := range rep.Repos {
		planeCol := "-"
		if r.Link != nil {
			planeCol = r.Link.Identifier
		} else if r.Editable {
			planeCol = "MISSING"
		}
		syn := strings.Join(r.Synapses, ",")
		if syn == "" {
			syn = "-"
		}
		fmt.Fprintf(tw, "%s\t%v\t%s\t%s\n", r.Name, r.Editable, planeCol, syn)
	}
	_ = tw.Flush()

	if len(rep.MissingProject) > 0 {
		fmt.Fprintln(out, "\n[-] Editable repositories without a Plane project:")
		for _, r := range rep.MissingProject {
			fmt.Fprintf(out, "    dendrite chronos project ensure %s\n", r.Root)
		}
	}
	if len(rep.Unsynapsed) > 0 {
		fmt.Fprintln(out, "\n[-] Related repositories that share no synapse:")
		for _, e := range rep.Unsynapsed {
			fmt.Fprintf(out, "    %s -> %s (via %s)\n", filepath.Base(e.From), filepath.Base(e.To), e.Via)
		}
	}
	if len(rep.MissingProject) == 0 && len(rep.Unsynapsed) == 0 {
		fmt.Fprintln(out, "\n[✓] Every editable repository is a Plane project and every related pair shares a synapse.")
	}
}
