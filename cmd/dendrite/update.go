package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"Dendrite/pkg/projectid"
	"Dendrite/pkg/selfupdate"
	"Dendrite/pkg/synapse"

	"github.com/spf13/cobra"
)

// releaseRepo is the GitHub repository dendrite releases are published to.
const releaseRepo = "yasuomaidana/dendrite"

// updateOptions are the parsed `dendrite update` flags.
type updateOptions struct {
	dev       bool
	sourceDir string
	target    string
	synapses  []string
	noSync    bool
	noMembers bool
	autostash bool
}

func (o *updateOptions) bind(cmd *cobra.Command) {
	f := cmd.Flags()
	f.BoolVar(&o.dev, "dev", false, "Build from local sources instead of downloading a release")
	f.StringVar(&o.sourceDir, "source-dir", "", "Dendrite checkout to build (default: $DENDRITE_SRC or the enclosing git repo)")
	f.StringVar(&o.target, "target", "", "Binary to replace (default: the running dendrite, or $GOBIN/dendrite under go run)")
	f.StringArrayVar(&o.synapses, "synapse", nil, "Sync and build only this synapse's members (repeatable)")
	f.BoolVar(&o.noSync, "no-sync", false, "Do not fetch or fast-forward synapse members")
	f.BoolVar(&o.noMembers, "no-members", false, "Do not build sibling member binaries")
	f.BoolVar(&o.autostash, "autostash", false, "Stash uncommitted changes around member fast-forwards")
}

// newUpdateCmd builds `dendrite update`: replace the binary from a release, or rebuild it from sources.
func newUpdateCmd() *cobra.Command {
	var o updateOptions
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"upgrade", "self-update"},
		Short:   "Update dendrite from a release, or rebuild it and its synapse siblings from local sources (--dev)",
		Long: "Release mode (default): downloads the latest GitHub release for this platform,\n" +
			"verifies its SHA-256 checksum and swaps it in with a backup and smoke test. While no\n" +
			"release is published it runs 'dendrite install' (git, on the installed channel's branch).\n\n" +
			"Develop mode (--dev): syncs every synapse the Dendrite checkout belongs to\n" +
			"(fast-forwarding local members and linking go.work / cargo patches; a synapse with a\n" +
			"channel also switches clean members back to their tracked branch), builds the\n" +
			"binaries of the local members, then rebuilds and swaps dendrite itself.",
		GroupID: "cortex",
		Args:    noArgs,
		Example: "dendrite update\ndendrite update --dev\ndendrite update --dev --synapse dendrite-ecosystem --no-members",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), o, cmd.OutOrStdout())
		},
	}
	o.bind(cmd)
	return cmd
}

// runUpdate implements `dendrite update`.
func runUpdate(ctx context.Context, o updateOptions, out io.Writer) error {
	target, err := resolveUpdateTarget(o.target)
	if err != nil {
		return err
	}
	if o.dev {
		return runDevUpdate(ctx, o, target, out)
	}
	err = runReleaseUpdate(ctx, selfupdate.GitHubReleases{Repo: releaseRepo}, target, out)
	if errors.Is(err, selfupdate.ErrNoRelease) {
		// No published binaries yet: git is the distribution channel.
		fmt.Fprintln(out, "[i] No release published; installing from git ('dendrite install')")
		return runInstall(ctx, installOptions{target: target}, out)
	}
	return err
}

func runReleaseUpdate(ctx context.Context, gh selfupdate.GitHubReleases, target string, out io.Writer) error {
	rel, err := gh.Latest(ctx)
	if err != nil {
		if errors.Is(err, selfupdate.ErrNoRelease) {
			return fmt.Errorf("%w; build from source with 'dendrite update --dev' inside a Dendrite checkout", err)
		}
		return err
	}
	fmt.Fprintf(out, "[+] Downloading %s (%s)\n", rel.Tag, rel.AssetURL)
	staged := selfupdate.StagePath(target)
	if err := gh.Download(ctx, rel, staged); err != nil {
		return err
	}
	if err := (selfupdate.Installer{Target: target}).Install(ctx, staged); err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] dendrite %s installed at %s (previous binary kept as %s.bak)\n", rel.Tag, target, target)
	return nil
}

func runDevUpdate(ctx context.Context, o updateOptions, target string, out io.Writer) error {
	src, err := resolveDendriteSource(o.sourceDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[+] Develop-mode update from %s\n", src)

	syns, err := updateSynapses(src, o.synapses)
	if err != nil {
		return err
	}
	if len(syns) == 0 {
		fmt.Fprintln(out, "[i] Dendrite belongs to no synapse; only dendrite itself is rebuilt ('dendrite synapse create' links sibling repos)")
	}

	st, err := synapse.DefaultStore()
	if err != nil {
		return err
	}
	var warnings []string
	if !o.noSync {
		for _, s := range syns {
			// A synapse on a channel (dendrite install) keeps every member on its tracked branch.
			rep := synapse.SyncSynapse(ctx, s, synapse.SyncOptions{Pull: true, Link: true, Autostash: o.autostash, Enforce: s.Channel != ""})
			if rep.Changed {
				if err := st.Save(s); err != nil {
					warnings = append(warnings, fmt.Sprintf("saving synapse %s: %v", s.ID, err))
				}
			}
			printSynapseSyncReport(out, rep)
			for _, m := range rep.Members {
				if m.Error != "" {
					warnings = append(warnings, fmt.Sprintf("%s/%s: %s", s.ID, m.Alias, m.Error))
				}
			}
		}
	}

	if !o.noMembers {
		w, err := buildSynapseMembers(ctx, syns, src, out)
		if err != nil {
			return err
		}
		warnings = append(warnings, w...)
	}

	if err := rebuildDendrite(ctx, src, target, out); err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintf(out, "    [!] %s\n", w)
	}
	return nil
}

// buildSynapseMembers builds the binaries of every local member of syns, except the
// Dendrite checkout at src, into $DENDRITE_HOME/bin. Build failures come back as warnings.
func buildSynapseMembers(ctx context.Context, syns []*synapse.Synapse, src string, out io.Writer) ([]string, error) {
	home, err := projectid.HomeDir()
	if err != nil {
		return nil, err
	}
	binDir := filepath.Join(home, "bin")
	var warnings []string
	built := map[string]bool{src: true}
	for _, s := range syns {
		for _, m := range s.Members {
			dir, err := synapse.Resolve(m)
			if m.IsRemote() || err != nil || built[dir] {
				continue
			}
			built[dir] = true
			art, outputs, err := selfupdate.BuildMember(ctx, dir, binDir)
			switch {
			case err != nil:
				warnings = append(warnings, fmt.Sprintf("building %s: %v", m.Alias, err))
				fmt.Fprintf(out, "    [-] %s: build failed\n", m.Alias)
			case len(outputs) == 0:
				fmt.Fprintf(out, "    [i] %s: %s, nothing to build\n", m.Alias, art.Kind)
			default:
				fmt.Fprintf(out, "    [✓] %s: built %s\n", m.Alias, strings.Join(outputs, ", "))
			}
		}
	}
	return warnings, nil
}

// rebuildDendrite builds cmd/dendrite from the checkout at src and swaps it in at target,
// keeping the previous binary as <target>.bak.
func rebuildDendrite(ctx context.Context, src, target string, out io.Writer) error {
	fmt.Fprintf(out, "[+] Building dendrite into %s\n", target)
	staged := selfupdate.StagePath(target)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := selfupdate.BuildGo(ctx, src, "./cmd/dendrite", staged); err != nil {
		_ = os.Remove(staged)
		return err
	}
	if err := (selfupdate.Installer{Target: target}).Install(ctx, staged); err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] dendrite rebuilt at %s (previous binary kept as %s.bak)\n", target, target)
	return nil
}

// resolveDendriteSource returns the Dendrite checkout to build: dir, $DENDRITE_SRC, or
// the git repository enclosing the working directory. It must hold `module Dendrite`
// and cmd/dendrite.
func resolveDendriteSource(dir string) (string, error) {
	if dir == "" {
		dir = os.Getenv("DENDRITE_SRC")
	}
	if dir == "" {
		top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			return "", errors.New("not inside a git checkout: pass --source-dir or set DENDRITE_SRC to a Dendrite checkout")
		}
		dir = strings.TrimSpace(string(top))
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if goModModule(filepath.Join(abs, "go.mod")) != "Dendrite" {
		return "", fmt.Errorf("%s is not a Dendrite checkout (go.mod must declare module Dendrite)", abs)
	}
	if info, err := os.Stat(filepath.Join(abs, "cmd", "dendrite")); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s has no cmd/dendrite", abs)
	}
	return abs, nil
}

// goModModule returns the module path declared in a go.mod, or "".
func goModModule(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

// updateSynapses returns the synapses named by ids, or every synapse with a local member
// at src when ids is empty.
func updateSynapses(src string, ids []string) ([]*synapse.Synapse, error) {
	st, err := synapse.DefaultStore()
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		var out []*synapse.Synapse
		for _, id := range ids {
			s, err := st.Load(id)
			if err != nil {
				return nil, fmt.Errorf("synapse %q: %w", id, err)
			}
			out = append(out, s)
		}
		return out, nil
	}
	all, err := st.List()
	if err != nil {
		return nil, err
	}
	var out []*synapse.Synapse
	for _, s := range all {
		if s.State == synapse.StateArchived {
			continue
		}
		for _, m := range s.Members {
			if dir, err := synapse.Resolve(m); err == nil && dir == src {
				out = append(out, s)
				break
			}
		}
	}
	return out, nil
}

// resolveUpdateTarget returns the binary to replace: explicit, else the running
// executable, unless that is a `go run` / test build in a temporary directory, in which
// case $GOBIN/dendrite or $GOPATH/bin/dendrite.
func resolveUpdateTarget(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	exe, err := os.Executable()
	if err == nil {
		if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
			exe = resolved
		}
		tmp, _ := filepath.EvalSymlinks(os.TempDir())
		if filepath.Base(exe) == "dendrite" && (tmp == "" || !strings.HasPrefix(exe, tmp)) {
			return exe, nil
		}
	}
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		return filepath.Join(gobin, "dendrite"), nil
	}
	gopath, err := exec.Command("go", "env", "GOPATH").Output()
	if err != nil || strings.TrimSpace(string(gopath)) == "" {
		return "", errors.New("cannot determine install target: pass --target")
	}
	return filepath.Join(filepath.SplitList(strings.TrimSpace(string(gopath)))[0], "bin", "dendrite"), nil
}

// buildRevision describes the VCS state the binary was built from, e.g.
// "commit 1a2b3c4 (2026-10-18T20:00:00Z, modified)", or "" when unknown.
func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return formatRevision(info.Settings)
}

func formatRevision(settings []debug.BuildSetting) string {
	var rev, at string
	modified := false
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			at = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev == "" {
		return ""
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	var extra []string
	if at != "" {
		extra = append(extra, at)
	}
	if modified {
		extra = append(extra, "modified")
	}
	if len(extra) == 0 {
		return "commit " + rev
	}
	return fmt.Sprintf("commit %s (%s)", rev, strings.Join(extra, ", "))
}
