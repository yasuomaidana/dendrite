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

	"github.com/spf13/cobra"
)

// installOptions are the parsed `dendrite install` flags.
type installOptions struct {
	channel synapse.Channel
	root    string
	refs    map[string]string
	target  string
	noBuild bool
}

// installFlags are the raw `dendrite install` flags.
type installFlags struct {
	dev, stable, noBuild bool
	root, target         string
	refs                 []string
}

func (f *installFlags) bind(cmd *cobra.Command) {
	fs := cmd.Flags()
	fs.BoolVar(&f.dev, "dev", false, "Track develop (the channel is remembered for later installs and 'update --dev')")
	fs.BoolVar(&f.stable, "stable", false, "Track main (default for a first install)")
	fs.StringVar(&f.root, "root", "", "Where members are cloned (default: the directory holding this Dendrite checkout, else ~/dendrite)")
	fs.StringArrayVar(&f.refs, "ref", nil, "Track another branch or tag for one member as <alias>=<ref>; <alias>= clears it (repeatable)")
	fs.StringVar(&f.target, "target", "", "Binary to replace (default: the running dendrite, or $GOBIN/dendrite under go run)")
	fs.BoolVar(&f.noBuild, "no-build", false, "Only clone and check out; build nothing")
	cmd.MarkFlagsMutuallyExclusive("dev", "stable")
}

// options validates the flags into installOptions.
func (f installFlags) options() (installOptions, error) {
	o := installOptions{root: f.root, target: f.target, noBuild: f.noBuild, refs: map[string]string{}}
	switch {
	case f.dev:
		o.channel = synapse.ChannelDev
	case f.stable:
		o.channel = synapse.ChannelStable
	}
	for _, spec := range f.refs {
		alias, ref, ok := strings.Cut(spec, "=")
		if !ok || alias == "" {
			return o, usageErrorf("invalid --ref %q: expected <alias>=<branch>", spec)
		}
		o.refs[alias] = ref
	}
	return o, nil
}

// newInstallCmd builds `dendrite install` (alias bootstrap): the ecosystem from git, on one channel.
func newInstallCmd() *cobra.Command {
	var f installFlags
	cmd := &cobra.Command{
		Use:     "install",
		Aliases: []string{"bootstrap"},
		Short:   "Install dendrite and its sub-projects from git: main (stable) or develop (--dev), per-member --ref overrides",
		Long: "Installs Dendrite and its sub-projects from git. Every member of the dendrite-ecosystem\n" +
			"synapse is cloned next to the others (when missing) and put on its tracked branch:\n" +
			"  stable (default)  main in every repository\n" +
			"  dev (--dev)       develop in every repository\n" +
			"A member can track another branch with --ref <alias>=<branch> (--ref <alias>= clears it).\n" +
			"Branches are enforced: a clean checkout on another branch is switched back; a checkout\n" +
			"with uncommitted changes stops the install. Working copies in dendrite-work/ are left alone.\n" +
			"Then every member is built into $DENDRITE_HOME/bin and dendrite itself is rebuilt.",
		GroupID: "cortex",
		Args:    noArgs,
		Example: "dendrite install\ndendrite install --dev\ndendrite install --dev --ref rag-rs=feat/ann\ndendrite install --dev --ref rag-rs=",
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := f.options()
			if err != nil {
				return err
			}
			return runInstall(cmd.Context(), o, cmd.OutOrStdout())
		},
	}
	f.bind(cmd)
	return cmd
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
