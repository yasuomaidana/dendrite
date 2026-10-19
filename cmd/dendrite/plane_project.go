package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"Dendrite/pkg/projectaudit"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

const planeProjectUsage = `Usage:
  dendrite plane project ensure [dir] [--identifier ID] [--name NAME] [--description TEXT] [--import FILE]
      Adopt or create the repository's Plane project, converge its states, and write .dendrite/plane.json.
      --import loads a markdown checklist (e.g. DEV_STATUS.md) as issues, one module per milestone.
  dendrite plane project show [dir]
      Print the Plane project linked to the repository.
  dendrite plane project audit [--roots dir,...] [--owner name,...] [--json]
      Check the rule: every editable repository is a Plane project, and repositories that
      depend on each other share a synapse. Exits non-zero when a repository breaks it.`

// errAuditViolations signals that the audit found repositories breaking the project rule.
var errAuditViolations = errors.New("project rule violations found")

func handlePlaneProject(ctx context.Context, client *plane.Client, args []string) {
	if err := runPlaneProject(ctx, client, args, os.Stdout); err != nil {
		if !errors.Is(err, errAuditViolations) {
			fmt.Fprintf(os.Stderr, "[-] %v\n", err)
		}
		os.Exit(1)
	}
}

func runPlaneProject(ctx context.Context, client *plane.Client, args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(out, planeProjectUsage)
		return nil
	}
	switch args[0] {
	case "ensure":
		return planeProjectEnsure(ctx, client, args[1:], out)
	case "show":
		return planeProjectShow(args[1:], out)
	case "audit":
		return planeProjectAudit(args[1:], out)
	default:
		fmt.Fprintln(out, planeProjectUsage)
		return fmt.Errorf("unknown plane project action %q", args[0])
	}
}

func planeProjectEnsure(ctx context.Context, client *plane.Client, args []string, out io.Writer) error {
	dir, identifier, name, description, importFile := ".", "", "", "", ""
	for i := 0; i < len(args); i++ {
		flag := args[i]
		switch flag {
		case "--identifier", "--name", "--description", "--import":
			if i+1 >= len(args) {
				return fmt.Errorf("flag %s requires an argument", flag)
			}
			val := args[i+1]
			i++
			switch flag {
			case "--identifier":
				identifier = val
			case "--name":
				name = val
			case "--description":
				description = val
			case "--import":
				importFile = val
			}
		default:
			if strings.HasPrefix(flag, "-") {
				return fmt.Errorf("unknown flag: %s", flag)
			}
			dir = flag
		}
	}

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
	if _, err := idx.Register(root); err != nil {
		return err
	}
	return idx.Save()
}

func planeProjectShow(args []string, out io.Writer) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	link, root, err := plane.LinkForDir(dir)
	if err != nil {
		if root != "" {
			return fmt.Errorf("%s has no Plane project; run `dendrite plane project ensure %s`", root, root)
		}
		return err
	}
	fmt.Fprintf(out, "%s -> %s (%s) project_id=%s workspace=%s\n", root, link.Identifier, link.Name, link.ProjectID, link.Workspace)
	return nil
}

func planeProjectAudit(args []string, out io.Writer) error {
	var roots, owners []string
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--roots", "--owner":
			if i+1 >= len(args) {
				return fmt.Errorf("flag %s requires an argument", args[i])
			}
			vals := strings.Split(args[i+1], ",")
			if args[i] == "--roots" {
				for _, v := range vals {
					roots = append(roots, expandHome(v))
				}
			} else {
				owners = append(owners, vals...)
			}
			i++
		case "--json":
			asJSON = true
		default:
			return fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	var syns []*synapse.Synapse
	if st, err := synapse.DefaultStore(); err == nil {
		syns, _ = st.List()
	}
	if len(roots) == 0 {
		roots = defaultAuditRoots(syns)
	}
	if len(owners) == 0 {
		if cwdRoot, err := plane.FindRepoRoot("."); err == nil {
			if o := projectaudit.OwnerOf(projectid.NormalizeRemote(projectid.GitRemote(cwdRoot))); o != "" {
				owners = []string{o}
			}
		}
	}

	rep := projectaudit.Audit(projectaudit.Discover(roots), owners, syns)
	if asJSON {
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
			fmt.Fprintf(out, "    dendrite plane project ensure %s\n", r.Root)
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
