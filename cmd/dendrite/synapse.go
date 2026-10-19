package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

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
func runSynapseCreate(ctx context.Context, subArgs []string, out io.Writer) error {
	var (
		id            string
		name          string
		membersRaw    string
		contractSpecs []string
		syncStrategy  string
		parityCmds    = make(map[string]string)
	)

	for i := 0; i < len(subArgs); i++ {
		arg := subArgs[i]
		switch {
		case arg == "--members":
			if i+1 >= len(subArgs) {
				return errors.New("flag --members requires an argument")
			}
			membersRaw = subArgs[i+1]
			i++
		case strings.HasPrefix(arg, "--members="):
			membersRaw = strings.TrimPrefix(arg, "--members=")
		case arg == "--name":
			if i+1 >= len(subArgs) {
				return errors.New("flag --name requires an argument")
			}
			name = subArgs[i+1]
			i++
		case strings.HasPrefix(arg, "--name="):
			name = strings.TrimPrefix(arg, "--name=")
		case arg == "--contract":
			if i+1 >= len(subArgs) {
				return errors.New("flag --contract requires an argument")
			}
			contractSpecs = append(contractSpecs, subArgs[i+1])
			i++
		case strings.HasPrefix(arg, "--contract="):
			contractSpecs = append(contractSpecs, strings.TrimPrefix(arg, "--contract="))
		case arg == "--sync":
			if i+1 >= len(subArgs) {
				return errors.New("flag --sync requires an argument")
			}
			syncStrategy = subArgs[i+1]
			i++
		case strings.HasPrefix(arg, "--sync="):
			syncStrategy = strings.TrimPrefix(arg, "--sync=")
		case arg == "--parity":
			if i+1 >= len(subArgs) {
				return errors.New("flag --parity requires an argument")
			}
			pair := subArgs[i+1]
			i++
			k, v, ok := strings.Cut(pair, "=")
			if !ok || k == "" || v == "" {
				return fmt.Errorf("invalid parity command %q: expected alias=cmd", pair)
			}
			parityCmds[k] = v
		case strings.HasPrefix(arg, "--parity="):
			pair := strings.TrimPrefix(arg, "--parity=")
			k, v, ok := strings.Cut(pair, "=")
			if !ok || k == "" || v == "" {
				return fmt.Errorf("invalid parity command %q: expected alias=cmd", pair)
			}
			parityCmds[k] = v
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag: %s", arg)
		default:
			if id == "" {
				id = arg
			} else {
				return fmt.Errorf("unexpected argument: %s", arg)
			}
		}
	}

	if id == "" {
		return errors.New("synapse id is required")
	}
	if membersRaw == "" {
		return errors.New("flag --members is required")
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
func runSynapseList(ctx context.Context, subArgs []string, out io.Writer) error {
	jsonMode := false
	for _, arg := range subArgs {
		switch {
		case arg == "--json":
			jsonMode = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag: %s", arg)
		default:
			return fmt.Errorf("unexpected argument: %s", arg)
		}
	}

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
					Plane:     m.PlaneProject,
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
func runSynapseInspect(ctx context.Context, subArgs []string, out io.Writer) error {
	var id string
	jsonMode := false

	for _, arg := range subArgs {
		switch {
		case arg == "--json":
			jsonMode = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag: %s", arg)
		default:
			if id == "" {
				id = arg
			} else {
				return fmt.Errorf("unexpected argument: %s", arg)
			}
		}
	}

	if id == "" {
		return errors.New("synapse id is required")
	}

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
				Source:    string(m.Source),
				Remote:    m.Remote,
				Ref:       m.Ref,
				Module:    m.Module,
				LockedSHA: m.LockedSHA,
				Plane:     m.PlaneProject,
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

// runSynapseSetPlane binds (or with "-" clears) the Plane project identifier of one member.
func runSynapseSetPlane(_ context.Context, subArgs []string, out io.Writer) error {
	if len(subArgs) != 3 {
		return errors.New("usage: dendrite synapse set-plane <id> <alias> <PLANE-IDENT|->")
	}
	id, alias, ident := subArgs[0], subArgs[1], strings.TrimSpace(subArgs[2])
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

// runSynapseCommand executes a synapse subcommand writing human/JSON output to out; returns an error instead of exiting.
func runSynapseCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		printSynapseUsage(out)
		return fmt.Errorf("unknown synapse action %q", "")
	}

	action := args[0]
	subArgs := args[1:]
	switch action {
	case "create":
		return runSynapseCreate(ctx, subArgs, out)
	case "list", "ls":
		return runSynapseList(ctx, subArgs, out)
	case "inspect":
		return runSynapseInspect(ctx, subArgs, out)
	case "heal":
		return runSynapseHeal(ctx, subArgs, out)
	case "locate":
		return runSynapseLocate(ctx, subArgs, out)
	case "move", "mv":
		return runSynapseMove(ctx, subArgs, out)
	case "ide-sync":
		return runSynapseIDESync(ctx, subArgs, out)
	case "set-source":
		return runSynapseSetSource(ctx, subArgs, out)
	case "channel":
		return runSynapseChannel(ctx, subArgs, out)
	case "status":
		return runSynapseSyncCommand(ctx, subArgs, out, false)
	case "sync":
		return runSynapseSyncCommand(ctx, subArgs, out, true)
	case "set-plane":
		return runSynapseSetPlane(ctx, subArgs, out)
	default:
		return fmt.Errorf("unknown synapse action %q", action)
	}
}

// handleSynapseCommand calls runSynapseCommand(ctx, args, os.Stdout); on error prints "[-] <err>" to stderr and os.Exit(1).
func handleSynapseCommand(ctx context.Context, args []string) {
	if err := runSynapseCommand(ctx, args, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[-] %v\n", err)
		os.Exit(1)
	}
}

// printSynapseUsage prints human-readable usage instructions for the synapse command.
func printSynapseUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: dendrite synapse <action> [flags...]")
	fmt.Fprintln(out, "\nAvailable Subcommands:")
	fmt.Fprintln(out, "  create <id> --members <dir>[=alias][:role][@PLANE],... [flags...]")
	fmt.Fprintln(out, "  set-plane <id> <alias> <PLANE-IDENT|->")
	fmt.Fprintln(out, "  list [--json]")
	fmt.Fprintln(out, "  inspect <id> [--json]")
	fmt.Fprintln(out, "  heal [--auto] [--roots dir,...] [--depth N] [--json]")
	fmt.Fprintln(out, "  locate <alias|project-id> [hint-dir...] [--json]")
	fmt.Fprintln(out, "  move <alias|project-id> <new-path> [--json]")
	fmt.Fprintln(out, "  ide-sync <id> [--binary path]")
	fmt.Fprintln(out, "  set-source <id> <alias> local|remote [--ref branch|tag|sha] [--remote url] [--module path]")
	fmt.Fprintln(out, "  status <id> [--no-fetch] [--json]")
	fmt.Fprintln(out, "  channel <id> [stable|dev]   Show or set the branch members track (stable: main, dev: develop)")
	fmt.Fprintln(out, "  sync <id> [--enforce] [--autostash] [--rebase] [--merge alias=branch]... [--no-link] [--no-fetch] [--json]")
	fmt.Fprintln(out, "  (scoped MCP server: dendrite mcp serve --synapse <id>)")
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

// runSynapseChannel shows or sets the channel whose branch members track by default.
// Members with their own --ref (set-source) keep it; the next 'synapse sync --enforce'
// moves the rest onto the new branch.
func runSynapseChannel(ctx context.Context, subArgs []string, out io.Writer) error {
	if len(subArgs) < 1 || len(subArgs) > 2 {
		return errors.New("usage: dendrite synapse channel <id> [stable|dev]")
	}
	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	s, err := st.Load(subArgs[0])
	if err != nil {
		return fmt.Errorf("synapse %q: %w", subArgs[0], err)
	}
	if len(subArgs) == 2 {
		ch, err := synapse.ParseChannel(subArgs[1])
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

// runSynapseSetSource switches a member between its local checkout and its remote
// for develop mode, optionally pinning the ref to track. The lock is cleared
// whenever source or ref change so the next sync re-resolves it.
func runSynapseSetSource(ctx context.Context, subArgs []string, out io.Writer) error {
	var positional []string
	var ref, remote, module *string
	for i := 0; i < len(subArgs); i++ {
		arg := subArgs[i]
		var target **string
		name := arg
		if eq := strings.Index(arg, "="); eq >= 0 && strings.HasPrefix(arg, "--") {
			name = arg[:eq]
		}
		switch name {
		case "--ref":
			target = &ref
		case "--remote":
			target = &remote
		case "--module":
			target = &module
		default:
			if strings.HasPrefix(arg, "-") {
				return fmt.Errorf("unknown flag: %s", arg)
			}
			positional = append(positional, arg)
			continue
		}
		var val string
		if eq := strings.Index(arg, "="); eq >= 0 {
			val = arg[eq+1:]
		} else {
			if i+1 >= len(subArgs) {
				return fmt.Errorf("%s requires a value", name)
			}
			i++
			val = subArgs[i]
		}
		*target = &val
	}
	if len(positional) != 3 {
		return errors.New("usage: dendrite synapse set-source <id> <alias> local|remote [--ref X] [--remote url] [--module path]")
	}
	id, alias, source := positional[0], positional[1], synapse.Source(positional[2])
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
