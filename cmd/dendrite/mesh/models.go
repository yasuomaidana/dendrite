package mesh

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/dotenv"
	"Dendrite/pkg/modelcatalog"
	"Dendrite/pkg/projectid"
	"Dendrite/pkg/rag"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// newModelsCmd builds `dendrite cleft models`: the model catalog that replaces hardcoded model names.
func newModelsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "models",
		Short: "Show the model catalog and which model plays each role (local, cheap, strong, copilot)",
		Long: "Models are found by asking the gateway, Ollama, `agy models` and the Gemini and Copilot listing APIs,\n" +
			"kept in $DENDRITE_HOME/model-catalog.json and refreshed at least weekly. Override a role with\n" +
			"DENDRITE_MODEL_<ROLE> or [roles] in $DENDRITE_HOME/models.toml.",
		Args: cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := catalogManager()
			if err != nil {
				return err
			}
			return runModelsList(cmd.OutOrStdout(), m)
		},
	}
	cmd.AddCommand(newModelsRefreshCmd(), newModelsScheduleCmd())
	return cmd
}

// catalogManager builds the manager a command uses; tests replace it.
var catalogManager = newCatalogManager

// newCatalogManager is the machine's manager with the gateway's local choice wired in.
func newCatalogManager() (*modelcatalog.Manager, error) {
	m, err := modelcatalog.Default()
	if err != nil {
		return nil, err
	}
	m.LocalPicker = func(ctx context.Context) (string, bool) {
		c, ok := rag.PreferLocalModel(ctx, dotenv.Lookup(m.Dir, "LITELLM_MASTER_KEY"))
		return c.Model, ok
	}
	return m, nil
}

func runModelsList(out io.Writer, m *modelcatalog.Manager) error {
	c, err := modelcatalog.Load(m.Dir)
	if err != nil {
		return err
	}
	if cli.Globals.JSON {
		return cli.WriteJSON(out, c)
	}
	if c.FetchedAt.IsZero() {
		fmt.Fprintln(out, "[i] No model catalog yet: run `dendrite cleft models refresh`.")
		return nil
	}
	age := m.Now().Sub(c.FetchedAt).Round(time.Minute)
	state := "fresh"
	if c.Stale(m.Now(), m.TTL) {
		state = "STALE, refresh with `dendrite cleft models refresh`"
	}
	fmt.Fprintf(out, "[+] Model catalog (%s old, %s)\n", age, state)
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ROLE\tMODEL\t")
	for _, role := range []string{modelcatalog.RoleLocal, modelcatalog.RoleCheap, modelcatalog.RoleStrong, modelcatalog.RoleCopilot} {
		fmt.Fprintf(tw, "%s\t%s\t\n", role, orDash(m.Role(role, "")))
	}
	tw.Flush()
	fmt.Fprintln(out)
	tw = tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "MODEL\tPROVIDER\t$/M IN\tCONTEXT\tROUTED\tSOURCE\t")
	for _, model := range c.Models {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t\n", model.Name, model.Provider, price(model),
			orDash(fmt.Sprint(model.MaxInput/1000)+"k"), yesNo(model.Routed), model.Source)
	}
	tw.Flush()
	for _, n := range c.Notes {
		fmt.Fprintf(out, "[!] %s\n", n)
	}
	return nil
}

func orDash(s string) string {
	if s == "" || s == "0k" {
		return "-"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// price shows dollars per million input tokens; a model the gateway does not route has no known price.
func price(m modelcatalog.Model) string {
	switch {
	case !m.Routed:
		return "-"
	case m.InputCost == 0:
		return "free"
	}
	return strconv.FormatFloat(math.Round(m.InputCost*1e9)/1e3, 'f', -1, 64)
}

func newModelsRefreshCmd() *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Ask every source for its models and rebuild the catalog",
		Args:  cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := catalogManager()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if quiet {
				out = io.Discard
			}
			return runModelsRefresh(cmd.Context(), out, m)
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "No output (used by the weekly job and background refreshes)")
	return cmd
}

func runModelsRefresh(ctx context.Context, out io.Writer, m *modelcatalog.Manager) error {
	c, err := m.Refresh(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] Catalog refreshed: %d models", len(c.Models))
	for _, role := range []string{modelcatalog.RoleLocal, modelcatalog.RoleCheap, modelcatalog.RoleStrong, modelcatalog.RoleCopilot} {
		if v := c.Roles[role]; v != "" {
			fmt.Fprintf(out, "  %s=%s", role, v)
		}
	}
	fmt.Fprintln(out)
	for _, n := range c.Notes {
		fmt.Fprintf(out, "[!] %s\n", n)
	}
	return nil
}

func newModelsScheduleCmd() *cobra.Command {
	var remove, noLoad bool
	var agentsDir string
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Install (or --remove) the weekly LaunchAgent that refreshes the catalog",
		Long:  "Writes ~/Library/LaunchAgents/dev.dendrite.models.plist (Mondays 09:00) and loads it. Where launchd is\nnot available the cron line to use is printed instead.",
		Args:  cli.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModelsSchedule(cmd.OutOrStdout(), agentsDir, remove, !noLoad && goos == "darwin")
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "Unload and delete the weekly job")
	cmd.Flags().BoolVar(&noLoad, "no-load", false, "Write the plist without loading it")
	cmd.Flags().StringVar(&agentsDir, "agents-dir", "", "LaunchAgents directory (default ~/Library/LaunchAgents)")
	_ = cmd.Flags().MarkHidden("agents-dir")
	return cmd
}

func runModelsSchedule(out io.Writer, agentsDir string, remove, load bool) error {
	exe, err := os.Executable()
	if err != nil {
		exe = "dendrite"
	}
	switch {
	case goos != "darwin" && agentsDir == "":
		fmt.Fprintf(out, "[i] launchd is not available here; add to crontab:\n    0 9 * * 1 %s cleft models refresh --quiet\n", exe)
		return nil
	case remove:
		return removeSchedule(out, agentsDir, load)
	}
	return installSchedule(out, agentsDir, exe, load)
}

func removeSchedule(out io.Writer, agentsDir string, load bool) error {
	path, err := modelcatalog.PlistPath(agentsDir)
	if err != nil {
		return err
	}
	if load {
		_, _ = launchctl("bootout", launchDomain(), path)
	}
	if _, err := modelcatalog.RemoveSchedule(agentsDir); err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] Weekly model refresh removed (%s)\n", path)
	return nil
}

func installSchedule(out io.Writer, agentsDir, exe string, load bool) error {
	logDir := os.TempDir()
	if home, err := projectid.HomeDir(); err == nil {
		logDir = filepath.Join(home, "logs")
		_ = os.MkdirAll(logDir, 0o755)
	}
	path, err := modelcatalog.InstallSchedule(agentsDir, exe, logDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] Weekly model refresh written: %s (Mondays 09:00)\n", path)
	if !load {
		return nil
	}
	_, _ = launchctl("bootout", launchDomain(), path)
	if msg, err := launchctl("bootstrap", launchDomain(), path); err != nil {
		return fmt.Errorf("launchctl bootstrap: %s", strings.TrimSpace(string(msg)))
	}
	fmt.Fprintln(out, "[✓] Loaded into launchd")
	return nil
}

func launchDomain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

// launchctl and goos are variables so tests never talk to the real launchd.
var (
	launchctl = func(args ...string) ([]byte, error) { return exec.Command("launchctl", args...).CombinedOutput() }
	goos      = runtime.GOOS
)

// refreshCatalogIfStale keeps the catalog within its weekly TTL from a daemon tick.
var refreshCatalogIfStale = func(ctx context.Context, out io.Writer) {
	if m, err := catalogManager(); err == nil {
		refreshIfStale(ctx, out, m)
	}
}

func refreshIfStale(ctx context.Context, out io.Writer, m *modelcatalog.Manager) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if refreshed, err := m.RefreshIfStale(ctx); err != nil {
		fmt.Fprintf(out, "[-] model catalog refresh: %v\n", err)
	} else if refreshed {
		fmt.Fprintln(out, "[i] model catalog refreshed")
	}
}
