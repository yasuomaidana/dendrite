package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/synapse"
)

type memberJSON struct {
	Alias     string `json:"alias"`
	ProjectID string `json:"project_id"`
	Role      string `json:"role"`
	PathHint  string `json:"path_hint"`
	Status    string `json:"status"`
}

type synapseListJSON struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	State   string       `json:"state"`
	Members []memberJSON `json:"members"`
}

type synapseInspectJSON struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	State     string             `json:"state"`
	Members   []memberJSON       `json:"members"`
	Contracts []synapse.Contract `json:"contracts"`
	Sync      synapse.Sync       `json:"sync"`
	Parity    synapse.Parity     `json:"parity"`
}

// synapseHealth returns "ok" if all members have status "ok", or "<n> unresolved" otherwise.
func synapseHealth(s *synapse.Synapse) string {
	unresolved := 0
	for _, m := range s.Members {
		if synapse.StatusOf(m) != synapse.StatusOK {
			unresolved++
		}
	}
	if unresolved == 0 {
		return "ok"
	}
	return fmt.Sprintf("%d unresolved", unresolved)
}

// parseMemberSpec parses a member descriptor of the form:
// <dir>[=alias][:role]
//
// Defaults: alias = filepath.Base(absDir), role = RolePeer.
func parseMemberSpec(spec string) (dir string, alias string, role synapse.Role, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", "", "", errors.New("empty member specification")
	}
	role = synapse.RolePeer

	if eqIdx := strings.Index(spec, "="); eqIdx >= 0 {
		dir = strings.TrimSpace(spec[:eqIdx])
		rest := strings.TrimSpace(spec[eqIdx+1:])
		if colonIdx := strings.Index(rest, ":"); colonIdx >= 0 {
			alias = strings.TrimSpace(rest[:colonIdx])
			role = synapse.Role(strings.ToLower(strings.TrimSpace(rest[colonIdx+1:])))
		} else {
			alias = rest
		}
	} else {
		if colonIdx := strings.LastIndex(spec, ":"); colonIdx >= 0 {
			dir = strings.TrimSpace(spec[:colonIdx])
			role = synapse.Role(strings.ToLower(strings.TrimSpace(spec[colonIdx+1:])))
		} else {
			dir = spec
		}
	}

	if dir == "" {
		return "", "", "", errors.New("member directory cannot be empty")
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to determine absolute path for %q: %w", dir, err)
	}

	if alias == "" {
		alias = filepath.Base(absDir)
	}

	return absDir, alias, role, nil
}

// parseContractSpec parses a contract descriptor of the form:
// kind:producerAlias:path[>consumerAlias:path]...
func parseContractSpec(raw string) (synapse.Contract, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return synapse.Contract{}, errors.New("empty contract specification")
	}
	parts := strings.Split(raw, ">")
	producerPart := strings.TrimSpace(parts[0])
	pFields := strings.SplitN(producerPart, ":", 3)
	if len(pFields) != 3 || pFields[0] == "" || pFields[1] == "" || pFields[2] == "" {
		return synapse.Contract{}, fmt.Errorf("invalid contract syntax %q: expected kind:producer:path[>consumer:path...]", raw)
	}

	contract := synapse.Contract{
		Kind:      pFields[0],
		Producer:  pFields[1],
		Path:      pFields[2],
		Consumers: make([]synapse.Consumer, 0, len(parts)-1),
	}

	for _, cPart := range parts[1:] {
		cPart = strings.TrimSpace(cPart)
		if cPart == "" {
			return synapse.Contract{}, fmt.Errorf("invalid contract syntax %q: empty consumer after '>'", raw)
		}
		cFields := strings.SplitN(cPart, ":", 2)
		if len(cFields) != 2 || cFields[0] == "" || cFields[1] == "" {
			return synapse.Contract{}, fmt.Errorf("invalid contract consumer syntax %q: expected alias:path", cPart)
		}
		contract.Consumers = append(contract.Consumers, synapse.Consumer{
			Alias: cFields[0],
			Path:  cFields[1],
		})
	}

	return contract, nil
}

// runSynapseCreate executes the create subcommand.
// synapseCreateOptions holds the flags of `synapse create`.
type synapseCreateOptions struct {
	Name      string
	Members   string
	Contracts []string
	Sync      string
	Parity    []string
}

func runSynapseCreate(ctx context.Context, id string, o synapseCreateOptions, out io.Writer) error {
	name, membersRaw, contractSpecs, syncStrategy := o.Name, o.Members, o.Contracts, o.Sync
	parityCmds := make(map[string]string)
	for _, pair := range o.Parity {
		k, v, ok := strings.Cut(pair, "=")
		if !ok || k == "" || v == "" {
			return usageErrorf("invalid parity command %q: expected alias=cmd", pair)
		}
		parityCmds[k] = v
	}

	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}

	if existing, err := st.Load(id); err == nil && existing != nil {
		return fmt.Errorf("synapse %q already exists", id)
	}

	indexPath, err := projectid.DefaultIndexPath()
	if err != nil {
		return err
	}
	idx, err := projectid.LoadIndex(indexPath)
	if err != nil {
		return err
	}

	var members []synapse.Member
	for _, spec := range strings.Split(membersRaw, ",") {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		absDir, alias, role, err := parseMemberSpec(spec)
		if err != nil {
			return err
		}
		entry, err := idx.Register(absDir)
		if err != nil {
			return fmt.Errorf("register project %s: %w", absDir, err)
		}
		members = append(members, synapse.Member{
			ProjectID: entry.ID,
			Alias:     alias,
			Role:      role,
			PathHint:  absDir,
		})
	}

	var contracts []synapse.Contract
	for _, cSpec := range contractSpecs {
		c, err := parseContractSpec(cSpec)
		if err != nil {
			return err
		}
		contracts = append(contracts, c)
	}

	if name == "" {
		name = id
	}

	syn := &synapse.Synapse{
		ID:        id,
		Name:      name,
		State:     synapse.StateActive,
		Members:   members,
		Contracts: contracts,
		Sync: synapse.Sync{
			Strategy: syncStrategy,
		},
		Parity: synapse.Parity{
			Commands: parityCmds,
		},
	}

	if err := syn.Validate(); err != nil {
		return err
	}

	if err := idx.Save(); err != nil {
		return fmt.Errorf("failed to save project index: %w", err)
	}

	if err := st.Save(syn); err != nil {
		return err
	}

	manifestPath := filepath.Join(st.Dir(), syn.ID+".toml")
	fmt.Fprintf(out, "[✓] Synapse %s created with %d members\n", syn.ID, len(syn.Members))
	fmt.Fprintf(out, "    Manifest: %s\n", manifestPath)
	return nil
}

// runSynapseList executes the list subcommand.
func runSynapseList(ctx context.Context, jsonMode bool, out io.Writer) error {
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}

	synapses, err := st.List()
	if err != nil {
		return err
	}

	if len(synapses) == 0 {
		if jsonMode {
			fmt.Fprintln(out, "[]")
		} else {
			fmt.Fprintln(out, "[+] No synapses defined.")
		}
		return nil
	}

	if jsonMode {
		listJSON := make([]synapseListJSON, 0, len(synapses))
		for _, s := range synapses {
			membersJSON := make([]memberJSON, 0, len(s.Members))
			for _, m := range s.Members {
				membersJSON = append(membersJSON, memberJSON{
					Alias:     m.Alias,
					ProjectID: m.ProjectID,
					Role:      string(m.Role),
					PathHint:  m.PathHint,
					Status:    string(synapse.StatusOf(m)),
				})
			}
			listJSON = append(listJSON, synapseListJSON{
				ID:      s.ID,
				Name:    s.Name,
				State:   string(s.State),
				Members: membersJSON,
			})
		}
		data, err := json.MarshalIndent(listJSON, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATE\tMEMBERS\tHEALTH")
	for _, s := range synapses {
		var aliases []string
		for _, m := range s.Members {
			aliases = append(aliases, m.Alias)
		}
		health := synapseHealth(s)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.ID, s.State, strings.Join(aliases, ", "), health)
	}
	return w.Flush()
}

// runSynapseInspect executes the inspect subcommand.
func runSynapseInspect(ctx context.Context, id string, jsonMode bool, out io.Writer) error {
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}

	s, err := st.Load(id)
	if err != nil {
		if errors.Is(err, synapse.ErrNotFound) {
			return fmt.Errorf("synapse %q: %w", id, synapse.ErrNotFound)
		}
		return err
	}

	if jsonMode {
		membersJSON := make([]memberJSON, 0, len(s.Members))
		for _, m := range s.Members {
			membersJSON = append(membersJSON, memberJSON{
				Alias:     m.Alias,
				ProjectID: m.ProjectID,
				Role:      string(m.Role),
				PathHint:  m.PathHint,
				Status:    string(synapse.StatusOf(m)),
			})
		}
		contracts := s.Contracts
		if contracts == nil {
			contracts = []synapse.Contract{}
		}
		parity := s.Parity
		if parity.Commands == nil {
			parity.Commands = make(map[string]string)
		}
		inspectJSON := synapseInspectJSON{
			ID:        s.ID,
			Name:      s.Name,
			State:     string(s.State),
			Members:   membersJSON,
			Contracts: contracts,
			Sync:      s.Sync,
			Parity:    parity,
		}
		data, err := json.MarshalIndent(inspectJSON, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(data))
		return nil
	}

	fmt.Fprintf(out, "[+] Synapse %s:\n", s.ID)
	fmt.Fprintf(out, "    Name:          %s\n", s.Name)
	fmt.Fprintf(out, "    State:         %s\n", s.State)
	syncStrat := s.Sync.Strategy
	if syncStrat == "" {
		syncStrat = "none"
	}
	fmt.Fprintf(out, "    Sync Strategy: %s\n", syncStrat)
	fmt.Fprintf(out, "    Members (%d):\n", len(s.Members))
	for _, m := range s.Members {
		status := synapse.StatusOf(m)
		fmt.Fprintf(out, "      * %-15s [%s] (%s) -> %s\n", m.Alias, m.Role, status, m.PathHint)
	}
	if len(s.Contracts) > 0 {
		fmt.Fprintf(out, "    Contracts (%d):\n", len(s.Contracts))
		for _, c := range s.Contracts {
			var consumerStrs []string
			for _, cons := range c.Consumers {
				consumerStrs = append(consumerStrs, fmt.Sprintf("%s:%s", cons.Alias, cons.Path))
			}
			if len(consumerStrs) > 0 {
				fmt.Fprintf(out, "      * [%s] %s:%s -> %s\n", c.Kind, c.Producer, c.Path, strings.Join(consumerStrs, ", "))
			} else {
				fmt.Fprintf(out, "      * [%s] %s:%s\n", c.Kind, c.Producer, c.Path)
			}
		}
	}
	if len(s.Parity.Commands) > 0 {
		fmt.Fprintf(out, "    Parity Commands (%d):\n", len(s.Parity.Commands))
		keys := make([]string, 0, len(s.Parity.Commands))
		for k := range s.Parity.Commands {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(out, "      * %s = %s\n", k, s.Parity.Commands[k])
		}
	}
	return nil
}

// newSynapseCmd builds `dendrite synapse`: typed cross-project links.
func newSynapseCmd() *cobra.Command {
	cmd := newGroupCmd("synapse", "Typed cross-project links with scoped MCP servers and self-healing paths", "cleft", "syn", "synapses")
	cmd.Example = "dendrite synapse create billing --members ../api=api:producer,../web=web:consumer\ndendrite synapse list --json\ndendrite synapse heal --auto"

	var create synapseCreateOptions
	createCmd := &cobra.Command{
		Use:     "create <id>",
		Short:   "Create a synapse manifest linking member projects",
		Args:    exactArgs(1),
		Example: "dendrite synapse create billing --members ../api=api:producer,../web=web:consumer --contract openapi:api:spec.yaml>web:client.ts",
		RunE: func(c *cobra.Command, args []string) error {
			return runSynapseCreate(c.Context(), args[0], create, c.OutOrStdout())
		},
	}
	cf := createCmd.Flags()
	cf.StringVar(&create.Members, "members", "", "Members as <dir>[=alias][:role],...")
	cf.StringVar(&create.Name, "name", "", "Display name (default: id)")
	cf.StringArrayVar(&create.Contracts, "contract", nil, "Contract kind:producer:path[>consumer:path...] (repeatable)")
	cf.StringVar(&create.Sync, "sync", "", "Sync strategy")
	cf.StringArrayVar(&create.Parity, "parity", nil, "Parity command alias=cmd (repeatable)")
	requireFlags(createCmd, "members")

	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List synapses and their health",
		Args:    noArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return runSynapseList(c.Context(), globals.JSON, c.OutOrStdout())
		},
	}

	inspectCmd := &cobra.Command{
		Use:   "inspect <id>",
		Short: "Show members, contracts and parity commands of a synapse",
		Args:  exactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runSynapseInspect(c.Context(), args[0], globals.JSON, c.OutOrStdout())
		},
	}

	var heal synapseHealOptions
	healCmd := &cobra.Command{
		Use:     "heal",
		Short:   "Find moved member projects and repair their paths",
		Args:    noArgs,
		Example: "dendrite synapse heal --roots ~/code,~/work --depth 3 --auto",
		RunE: func(c *cobra.Command, args []string) error {
			return runSynapseHeal(c.Context(), heal, globals.JSON, c.OutOrStdout())
		},
	}
	healCmd.Flags().BoolVar(&heal.Auto, "auto", false, "Apply the repairs instead of only reporting them")
	healCmd.Flags().StringSliceVar(&heal.Roots, "roots", nil, "Search roots (comma-separated, repeatable)")
	healCmd.Flags().IntVar(&heal.Depth, "depth", 0, "Maximum search depth")

	locateCmd := &cobra.Command{
		Use:   "locate <alias|project-id> [hint-dir...]",
		Short: "Locate a member project on disk",
		Args:  minArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runSynapseLocate(c.Context(), args[0], args[1:], globals.JSON, c.OutOrStdout())
		},
	}

	moveCmd := &cobra.Command{
		Use:     "move <alias|project-id> <new-path>",
		Aliases: []string{"mv"},
		Short:   "Move a member project and update every manifest atomically",
		Args:    exactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			return runSynapseMove(c.Context(), args[0], args[1], globals.JSON, c.OutOrStdout())
		},
	}

	var binary string
	ideSyncCmd := &cobra.Command{
		Use:   "ide-sync <id>",
		Short: "Write IDE MCP configuration for a synapse's members",
		Args:  exactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runSynapseIDESync(c.Context(), args[0], binary, c.OutOrStdout())
		},
	}
	ideSyncCmd.Flags().StringVar(&binary, "binary", "", "dendrite binary for the MCP entry (default: this executable)")

	cmd.AddCommand(createCmd, listCmd, inspectCmd, healCmd, locateCmd, moveCmd, ideSyncCmd)
	return cmd
}
