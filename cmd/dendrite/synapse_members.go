package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
	"Dendrite/pkg/tracker/plane"
)

// buildMembers parses a comma-separated member spec string, determines Plane project bindings,
// and registers each member in the project index.
func buildMembers(idx *projectid.Index, membersRaw string) ([]synapse.Member, error) {
	var members []synapse.Member
	for _, spec := range strings.Split(membersRaw, ",") {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		spec, planeProject := splitPlaneSuffix(spec)
		absDir, alias, role, err := parseMemberSpec(spec)
		if err != nil {
			return nil, err
		}
		if planeProject == "" {
			if link, _, err := plane.LinkForDir(absDir); err == nil && link != nil && link.Identifier != "" {
				planeProject = link.Identifier
			}
		}
		entry, err := idx.Register(absDir)
		if err != nil {
			return nil, fmt.Errorf("register project %s: %w", absDir, err)
		}
		members = append(members, synapse.Member{
			ProjectID:    entry.ID,
			Alias:        alias,
			Role:         role,
			PathHint:     absDir,
			PlaneProject: planeProject,
		})
	}
	return members, nil
}

// runSynapseAdd adds one or more member projects to an existing synapse.
func runSynapseAdd(_ context.Context, id, membersRaw string, out io.Writer) error {
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}

	syn, err := st.Load(id)
	if err != nil {
		if errors.Is(err, synapse.ErrNotFound) {
			return fmt.Errorf("no such synapse %q", id)
		}
		return err
	}

	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		return err
	}
	idx, err := projectid.LoadIndex(indexPath)
	if err != nil {
		return err
	}

	newMembers, err := buildMembers(idx, membersRaw)
	if err != nil {
		return err
	}
	if len(newMembers) == 0 {
		return errors.New("no members specified")
	}

	// Refuse any member whose alias or project ID is already in the synapse.
	for _, m := range newMembers {
		for _, existing := range syn.Members {
			if existing.Alias == m.Alias {
				return fmt.Errorf("synapse %q already has member with alias %q", id, m.Alias)
			}
			if existing.ProjectID == m.ProjectID {
				return fmt.Errorf("synapse %q already has member with project ID %q", id, m.ProjectID)
			}
		}
	}

	syn.Members = append(syn.Members, newMembers...)

	if err := syn.Validate(); err != nil {
		return err
	}

	if err := idx.Save(); err != nil {
		return fmt.Errorf("failed to save project index: %w", err)
	}

	if err := st.Save(syn); err != nil {
		return err
	}

	for _, m := range newMembers {
		fmt.Fprintf(out, "[+] Added %s (%s) to synapse %s\n", m.Alias, m.Role, id)
	}
	return nil
}

// runSynapseRemoveMember removes a member project (by alias or project ID) from a synapse.
func runSynapseRemoveMember(_ context.Context, id, memberRef string, out io.Writer) error {
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}

	syn, err := st.Load(id)
	if err != nil {
		if errors.Is(err, synapse.ErrNotFound) {
			return fmt.Errorf("no such synapse %q", id)
		}
		return err
	}

	targetIndex := -1
	for i, m := range syn.Members {
		if m.Alias == memberRef || m.ProjectID == memberRef {
			targetIndex = i
			break
		}
	}
	if targetIndex < 0 {
		return fmt.Errorf("synapse %q has no member %q", id, memberRef)
	}

	targetAlias := syn.Members[targetIndex].Alias

	// Check if a contract or a parity command references the alias
	for _, c := range syn.Contracts {
		if c.Producer == targetAlias {
			return fmt.Errorf("cannot remove member %q: referenced by contract %s (%s)", targetAlias, c.Kind, c.Path)
		}
		for _, cons := range c.Consumers {
			if cons.Alias == targetAlias {
				return fmt.Errorf("cannot remove member %q: referenced by contract %s (%s)", targetAlias, c.Kind, cons.Path)
			}
		}
	}

	if cmd, ok := syn.Parity.Commands[targetAlias]; ok {
		return fmt.Errorf("cannot remove member %q: referenced by parity command %q", targetAlias, cmd)
	}

	syn.Members = append(syn.Members[:targetIndex], syn.Members[targetIndex+1:]...)

	if err := syn.Validate(); err != nil {
		return err
	}

	if err := st.Save(syn); err != nil {
		return err
	}

	fmt.Fprintf(out, "[-] Removed %s from synapse %s\n", targetAlias, id)
	return nil
}

// isTerminal reports whether in is the process's interactive stdin; a seam for tests.
var isTerminal = func(in io.Reader) bool {
	f, ok := in.(*os.File)
	return ok && f == os.Stdin && stdinIsTerminal()
}

// runSynapseDelete deletes a synapse manifest by ID.
func runSynapseDelete(_ context.Context, id string, in io.Reader, out io.Writer) error {
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}

	if !globals.Yes {
		if !isTerminal(in) {
			return fmt.Errorf("cannot delete synapse %q in non-interactive mode; pass --yes to confirm", id)
		}
		fmt.Fprintf(out, "Delete synapse %q? [y/N]: ", id)
		reader := bufio.NewReader(in)
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return fmt.Errorf("deletion of synapse %q cancelled", id)
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line != "y" && line != "yes" {
			return fmt.Errorf("deletion of synapse %q cancelled", id)
		}
	}

	if err := st.Delete(id); err != nil {
		if errors.Is(err, synapse.ErrNotFound) {
			return fmt.Errorf("no such synapse %q", id)
		}
		return err
	}

	fmt.Fprintf(out, "[-] Deleted synapse %s\n", id)
	return nil
}
