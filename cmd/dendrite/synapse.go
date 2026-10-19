package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

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
	Source    string `json:"source,omitempty"`
	Remote    string `json:"remote,omitempty"`
	Ref       string `json:"ref,omitempty"`
	Module    string `json:"module,omitempty"`
	LockedSHA string `json:"locked_sha,omitempty"`
	Plane     string `json:"plane_project,omitempty"`
}

// toMemberJSON renders a member with its live status for the --json listings.
func toMemberJSON(m synapse.Member) memberJSON {
	return memberJSON{
		Alias:     m.Alias,
		ProjectID: m.ProjectID,
		Role:      string(m.Role),
		PathHint:  m.PathHint,
		Status:    string(synapse.StatusOf(m)),
		Source:    string(m.Source),
		Remote:    m.Remote,
		Ref:       m.Ref,
		Module:    m.Module,
		LockedSHA: m.LockedSHA,
		Plane:     m.PlaneProject,
	}
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
	Channel   string             `json:"channel,omitempty"`
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
		spec, planeProject := splitPlaneSuffix(spec)
		absDir, alias, role, err := parseMemberSpec(spec)
		if err != nil {
			return err
		}
		entry, err := idx.Register(absDir)
		if err != nil {
			return fmt.Errorf("register project %s: %w", absDir, err)
		}
		members = append(members, synapse.Member{
			ProjectID:    entry.ID,
			Alias:        alias,
			Role:         role,
			PathHint:     absDir,
			PlaneProject: planeProject,
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
				membersJSON = append(membersJSON, toMemberJSON(m))
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
			membersJSON = append(membersJSON, toMemberJSON(m))
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
			Channel:   string(s.Channel),
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
	if s.Channel != "" {
		fmt.Fprintf(out, "    Channel:       %s (tracks %s)\n", s.Channel, s.Channel.DefaultRef())
	}
	fmt.Fprintf(out, "    Members (%d):\n", len(s.Members))
	for _, m := range s.Members {
		status := synapse.StatusOf(m)
		line := fmt.Sprintf("      * %-15s [%s] (%s) -> %s%s", m.Alias, m.Role, status, m.PathHint, describeSource(m))
		if m.PlaneProject != "" {
			line += "  plane=" + m.PlaneProject
		}
		fmt.Fprintln(out, line)
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
	cf.StringVar(&create.Members, "members", "", "Members as <dir>[=alias][:role][@PLANE],...")
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

	setPlaneCmd := &cobra.Command{
		Use:     "set-plane <id> <alias> <PLANE-IDENT|->",
		Short:   "Bind a member to its Plane project identifier (- clears it)",
		Args:    exactArgs(3),
		Example: "dendrite synapse set-plane dendrite-ecosystem git-rs DGIT",
		RunE: func(c *cobra.Command, args []string) error {
			return runSynapseSetPlane(c.Context(), args[0], args[1], args[2], c.OutOrStdout())
		},
	}

	var ref, remote, module string
	setSourceCmd := &cobra.Command{
		Use:   "set-source <id> <alias> local|remote",
		Short: "Consume a member from its local checkout or its remote, optionally tracking another ref",
		Long: "Switches a member between its local checkout and its remote for develop mode. --ref sets the\n" +
			"branch, tag or commit the member tracks instead of the synapse channel's branch (--ref= clears it).",
		Args:    exactArgs(3),
		Example: "dendrite synapse set-source dendrite-ecosystem rag-rs local --ref feat/ann\ndendrite synapse set-source dendrite-ecosystem mcp-hub remote --ref develop",
		RunE: func(c *cobra.Command, args []string) error {
			pick := func(name string, v *string) *string {
				if c.Flags().Changed(name) {
					return v
				}
				return nil
			}
			return runSynapseSetSource(c.Context(), args[0], args[1], synapse.Source(args[2]),
				pick("ref", &ref), pick("remote", &remote), pick("module", &module), c.OutOrStdout())
		},
	}
	setSourceCmd.Flags().StringVar(&ref, "ref", "", "Branch, tag or commit to track (empty: the channel's branch)")
	setSourceCmd.Flags().StringVar(&remote, "remote", "", "Git URL of the member (default: its origin)")
	setSourceCmd.Flags().StringVar(&module, "module", "", "Go module path or Cargo crate name consumers link against")

	channelCmd := &cobra.Command{
		Use:     "channel <id> [stable|dev]",
		Short:   "Show or set the branch members track by default (stable: main, dev: develop)",
		Args:    rangeArgs(1, 2),
		Example: "dendrite synapse channel dendrite-ecosystem\ndendrite synapse channel dendrite-ecosystem dev",
		RunE: func(c *cobra.Command, args []string) error {
			return runSynapseChannel(c.Context(), args, c.OutOrStdout())
		},
	}

	cmd.AddCommand(createCmd, listCmd, inspectCmd, healCmd, locateCmd, moveCmd, ideSyncCmd,
		setPlaneCmd, setSourceCmd, channelCmd, newSynapseStatusCmd(), newSynapseSyncCmd())
	return cmd
}

var planeIdentRegex = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,11}$`)

// splitPlaneSuffix strips a trailing "@IDENT" Plane project identifier from a member spec.
func splitPlaneSuffix(spec string) (rest string, planeProject string) {
	at := strings.LastIndex(spec, "@")
	if at < 0 {
		return spec, ""
	}
	ident := strings.TrimSpace(spec[at+1:])
	if !planeIdentRegex.MatchString(ident) {
		return spec, ""
	}
	return spec[:at], strings.ToUpper(ident)
}

// runSynapseSetPlane binds (or with "-" clears) the Plane project identifier of one member.
func runSynapseSetPlane(_ context.Context, id, alias, ident string, out io.Writer) error {
	ident = strings.TrimSpace(ident)
	if ident == "-" {
		ident = ""
	} else if !planeIdentRegex.MatchString(ident) {
		return fmt.Errorf("invalid Plane project identifier %q", ident)
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	syn, err := st.Load(id)
	if err != nil {
		return err
	}
	found := false
	for i := range syn.Members {
		if syn.Members[i].Alias == alias || syn.Members[i].ProjectID == alias {
			syn.Members[i].PlaneProject = strings.ToUpper(ident)
			found = true
		}
	}
	if !found {
		return fmt.Errorf("synapse %q has no member %q", id, alias)
	}
	if err := st.Save(syn); err != nil {
		return err
	}
	if ident == "" {
		fmt.Fprintf(out, "[✓] Cleared Plane project for %s in synapse %s\n", alias, id)
	} else {
		fmt.Fprintf(out, "[✓] Bound %s in synapse %s to Plane project %s\n", alias, id, strings.ToUpper(ident))
	}
	return nil
}

// describeSource renders a member's develop-mode source for the inspect listing,
// e.g. " {remote develop @abc1234}"; local members tracking their current branch render as "".
func describeSource(m synapse.Member) string {
	var parts []string
	if m.IsRemote() {
		parts = append(parts, "remote")
	}
	if m.Ref != "" {
		parts = append(parts, m.Ref)
	}
	if m.LockedSHA != "" {
		sha := m.LockedSHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		parts = append(parts, "@"+sha)
	}
	if len(parts) == 0 {
		return ""
	}
	return " {" + strings.Join(parts, " ") + "}"
}

// runSynapseSetSource switches a member between its local checkout and its remote
// for develop mode, optionally pinning the ref to track (nil pointers leave a field as is).
// The lock is cleared whenever source or ref change so the next sync re-resolves it.
func runSynapseSetSource(ctx context.Context, id, alias string, source synapse.Source, ref, remote, module *string, out io.Writer) error {
	if source != synapse.SourceLocal && source != synapse.SourceRemote {
		return fmt.Errorf("invalid source %q (want local or remote)", source)
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	s, err := st.Load(id)
	if err != nil {
		return fmt.Errorf("synapse %q: %w", id, err)
	}
	m, ok := s.Member(alias)
	if !ok {
		return fmt.Errorf("synapse %q has no member %q", id, alias)
	}

	before := *m
	if remote != nil {
		m.Remote = *remote
	}
	if m.Remote == "" && m.PathHint != "" {
		// Keep the fetchable URL as configured; NormalizeRemote is only for identity matching.
		m.Remote = projectid.GitRemote(m.PathHint)
	}
	if ref != nil {
		m.Ref = *ref
	}
	if module != nil {
		m.Module = *module
	}
	m.Source = source
	if source == synapse.SourceLocal {
		if _, err := synapse.Resolve(*m); err != nil {
			return fmt.Errorf("member %q has no local checkout at %q: run 'dendrite synapse heal' first: %w", alias, m.PathHint, err)
		}
	}
	if m.Source != before.Source || m.Ref != before.Ref || m.Remote != before.Remote {
		m.LockedSHA, m.LockedAt = "", time.Time{}
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if err := st.Save(s); err != nil {
		return err
	}
	fmt.Fprintf(out, "[+] %s/%s now consumed from %s%s\n", s.ID, m.Alias, m.Source, describeSource(*m))
	return nil
}

// runSynapseChannel shows or sets the channel whose branch members track by default.
// Members with their own --ref (set-source) keep it; the next 'synapse sync --enforce'
// moves the rest onto the new branch.
func runSynapseChannel(_ context.Context, args []string, out io.Writer) error {
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	s, err := st.Load(args[0])
	if err != nil {
		return fmt.Errorf("synapse %q: %w", args[0], err)
	}
	if len(args) == 2 {
		ch, err := synapse.ParseChannel(args[1])
		if err != nil {
			return err
		}
		if ch != s.Channel {
			s.Channel = ch
			for i := range s.Members {
				s.Members[i].LockedSHA, s.Members[i].LockedAt = "", time.Time{}
			}
			if err := st.Save(s); err != nil {
				return err
			}
		}
	}
	if s.Channel == "" {
		fmt.Fprintf(out, "[i] %s has no channel: members follow their checkout's upstream\n", s.ID)
		return nil
	}
	fmt.Fprintf(out, "[+] %s channel: %s (tracks %s)\n", s.ID, s.Channel, s.Channel.DefaultRef())
	for _, m := range s.Members {
		if m.Ref != "" {
			fmt.Fprintf(out, "    [i] %s overrides it with %s\n", m.Alias, m.Ref)
		}
	}
	return nil
}
