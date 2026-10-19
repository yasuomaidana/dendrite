package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"Dendrite/pkg/ecosystem"
	"Dendrite/pkg/synapse"
)

// installOptions are the parsed `dendrite install` flags.
type installOptions struct {
	channel synapse.Channel
	root    string
	refs    map[string]string
	target  string
	noBuild bool
}

func printInstallUsage(out io.Writer) {
	fmt.Fprintln(out, "Usage: dendrite install [--dev|--stable] [--root <dir>] [--ref <alias>=<branch>]... [--target <path>] [--no-build]")
	fmt.Fprintln(out, "\nInstalls Dendrite and its sub-projects from git. Every member of the dendrite-ecosystem")
	fmt.Fprintln(out, "synapse is cloned next to the others (when missing) and put on its tracked branch:")
	fmt.Fprintln(out, "  stable (default)  main in every repository")
	fmt.Fprintln(out, "  dev (--dev)       develop in every repository")
	fmt.Fprintln(out, "A member can track another branch with --ref <alias>=<branch> (--ref <alias>= clears it).")
	fmt.Fprintln(out, "Branches are enforced: a clean checkout on another branch is switched back; a checkout")
	fmt.Fprintln(out, "with uncommitted changes stops the install. Working copies in dendrite-work/ are left alone.")
	fmt.Fprintln(out, "Then every member is built into $DENDRITE_HOME/bin and dendrite itself is rebuilt.")
	fmt.Fprintln(out, "\nFlags:")
	fmt.Fprintln(out, "  --dev                Track develop (the channel is remembered for later installs and 'update --dev')")
	fmt.Fprintln(out, "  --stable             Track main (default for a first install)")
	fmt.Fprintln(out, "  --root <dir>         Where members are cloned (default: the directory holding this Dendrite checkout, else ~/dendrite)")
	fmt.Fprintln(out, "  --ref <alias>=<ref>  Track another branch or tag for one member (repeatable)")
	fmt.Fprintln(out, "  --target <path>      Binary to replace (default: the running dendrite, or $GOBIN/dendrite under go run)")
	fmt.Fprintln(out, "  --no-build           Only clone and check out; build nothing")
}

func parseInstallArgs(args []string) (installOptions, error) {
	o := installOptions{refs: map[string]string{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, val, hasVal := strings.Cut(arg, "=")
		takeValue := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", name)
			}
			i++
			return args[i], nil
		}
		var err error
		switch name {
		case "--dev":
			o.channel = synapse.ChannelDev
		case "--stable":
			o.channel = synapse.ChannelStable
		case "--no-build":
			o.noBuild = true
		case "--root":
			o.root, err = takeValue()
		case "--target":
			o.target, err = takeValue()
		case "--ref":
			var spec string
			if spec, err = takeValue(); err == nil {
				alias, ref, ok := strings.Cut(spec, "=")
				if !ok || alias == "" {
					return o, fmt.Errorf("invalid --ref %q: expected <alias>=<branch>", spec)
				}
				o.refs[alias] = ref
			}
		default:
			return o, fmt.Errorf("unknown flag: %s", arg)
		}
		if err != nil {
			return o, err
		}
	}
	return o, nil
}

// runInstallCommand implements `dendrite install`.
func runInstallCommand(ctx context.Context, args []string, out io.Writer) error {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			printInstallUsage(out)
			return nil
		}
	}
	o, err := parseInstallArgs(args)
	if err != nil {
		return err
	}
	return runInstall(ctx, o, out)
}

func runInstall(ctx context.Context, o installOptions, out io.Writer) error {
	store, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	def, err := ecosystem.Definition()
	if err != nil {
		return err
	}
	if o.channel == "" {
		// Re-running install keeps the channel a previous install chose.
		o.channel = synapse.ChannelStable
		if prev, err := store.Load(def.ID); err == nil && prev.Channel != "" {
			o.channel = prev.Channel
		}
	}

	dirs := map[string]string{}
	src, srcErr := resolveDendriteSource("")
	if srcErr == nil {
		dirs[ecosystem.ControlAlias] = src
	}
	if o.root == "" {
		if srcErr == nil {
			o.root = filepath.Dir(src)
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return errors.New("cannot find a home directory: pass --root")
			}
			o.root = filepath.Join(home, "dendrite")
		}
	}

	fmt.Fprintf(out, "[+] Installing %s on the %s channel (%s) under %s\n", def.ID, o.channel, o.channel.DefaultRef(), o.root)
	res, err := ecosystem.Install(ctx, ecosystem.Options{
		Root: o.root, Channel: o.channel, Refs: o.refs, Dirs: dirs, Definition: def, Store: store,
	})
	if res != nil {
		for _, m := range res.Members {
			action := "present"
			if m.Cloned {
				action = "cloned"
			}
			fmt.Fprintf(out, "    [✓] %-8s %-8s %s @ %s\n", m.Alias, action, m.Dir, m.Ref)
		}
		if res.Sync != nil {
			printSynapseSyncReport(out, res.Sync)
		}
	}
	if err != nil {
		return err
	}
	if o.noBuild {
		return nil
	}

	control, ok := res.Synapse.Member(ecosystem.ControlAlias)
	if !ok {
		return errors.New("the ecosystem has no control member")
	}
	target, err := resolveUpdateTarget(o.target)
	if err != nil {
		return err
	}
	warnings, err := buildSynapseMembers(ctx, []*synapse.Synapse{res.Synapse}, control.PathHint, out)
	if err != nil {
		return err
	}
	if err := rebuildDendrite(ctx, control.PathHint, target, out); err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintf(out, "    [!] %s\n", w)
	}
	return nil
}
