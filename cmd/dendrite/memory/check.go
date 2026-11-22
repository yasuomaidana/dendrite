package memory

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/engram"
	"Dendrite/pkg/launchd"
	"Dendrite/pkg/memory"
	"Dendrite/pkg/projectid"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	var dir string
	var quiet bool
	cmd := noArgsCmd("check", "Score Claude memory and Engram, and switch the memory mode when the scores say so", func(cmd *cobra.Command) error {
		return runCheck(cmd.Context(), dir, quiet, cmd.OutOrStdout())
	})
	cmd.Long = "Mirrors Claude's file memories into Engram so both hold the same facts, then scores each store: recall (how\n" +
		"many memories it brings back), what was written this week, how many repository paths it names that no longer\n" +
		"exist, and what it costs at session start. The scores point to a mode (both, engram or claude). The mode\n" +
		"changes only after two checks in a row agree, except that an Engram that is missing or unhealthy is switched\n" +
		"off at once. A mode set with `memory mode set` is never changed. Dendrite only controls its own writes and\n" +
		"the guidance it gives agents: Claude Code's file memory itself is never touched."
	cmd.Flags().StringVar(&dir, "dir", "", "Claude memory folder (default ~/.claude/projects/<repo>/memory)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Print nothing unless it fails (for the weekly job)")
	return cmd
}

func runCheck(ctx context.Context, dir string, quiet bool, out io.Writer) error {
	home, err := projectid.HomeDir()
	if err != nil {
		return err
	}
	if dir, err = claudeFolder(dir); err != nil {
		return err
	}
	repo := claudeRepo()
	in := memory.CheckInput{ClaudeDir: dir, Repo: repo, Now: time.Now()}
	if bin, err := engram.Locate(); err == nil {
		in.Store = memory.EngramStore{Bin: bin, Dir: repo}
		if _, err := memory.Mirror(ctx, dir); err != nil {
			return err
		}
	}
	report, err := memory.Check(ctx, in)
	if err != nil {
		return err
	}
	last, hasLast := memory.LastCheckBefore(home, in.Now.Add(-memory.MinGap))
	var previous *memory.Report
	if hasLast {
		previous = &last
	}
	state := memory.Decide(memory.LoadState(home), previous, report, in.Now)
	if err := memory.AppendCheck(home, report); err != nil {
		return err
	}
	if err := memory.SaveState(home, state); err != nil {
		return err
	}
	if quiet {
		return nil
	}
	return printCheck(out, report, state)
}

// printCheck reports the scores of both stores and the mode in force.
func printCheck(out io.Writer, r memory.Report, s memory.State) error {
	if cli.Globals.JSON {
		return cli.WriteJSON(out, map[string]any{"report": r, "mode": s.Resolve(), "state": s})
	}
	fmt.Fprintf(out, "[✓] Memory check %s\n", r.Time.Format("2006-01-02"))
	for _, row := range []struct {
		name string
		s    memory.Score
	}{{"Claude", r.Claude}, {"Engram", r.Engram}} {
		fmt.Fprintf(out, "    - %-6s score %.2f  recall %3.0f%%  new this week %d  stale %2.0f%%  memories %d  session start %d bytes\n",
			row.name, row.s.Score, row.s.Recall*100, row.s.New7d, row.s.Stale*100, row.s.Memories, row.s.StartBytes)
	}
	if r.Note != "" {
		fmt.Fprintf(out, "    - %s\n", r.Note)
	}
	fmt.Fprintf(out, "    - The scores point to: %s. Mode in force: %s", r.Candidate, s.Resolve())
	if s.Override != "" {
		fmt.Fprint(out, " (override)")
	}
	fmt.Fprintln(out)
	return nil
}

// executable is this binary's path (a seam for tests).
var executable = os.Executable

// goos and launchctl are variables so tests never talk to the real launchd.
var (
	goos                     = runtime.GOOS
	launchctl launchd.Runner = launchd.Exec
)

// memoryJob is the weekly check: Mondays at 09:30, after the model catalog refresh.
var memoryJob = launchd.Job{
	Label: "dev.dendrite.memory", LogName: "memory-check", Weekday: 1, Hour: 9, Minute: 30,
	Args: []string{"memory", "check", "--quiet"},
}

func newScheduleCmd() *cobra.Command {
	var remove, noLoad bool
	var agentsDir string
	cmd := noArgsCmd("schedule", "Install (or --remove) the weekly LaunchAgent that runs `memory check`", func(cmd *cobra.Command) error {
		return runSchedule(cmd.OutOrStdout(), agentsDir, remove, !noLoad && goos == "darwin")
	})
	cmd.Long = "Writes ~/Library/LaunchAgents/dev.dendrite.memory.plist (Mondays 09:30) and loads it. Where launchd is\n" +
		"not available the cron line to use is printed instead."
	cmd.Flags().BoolVar(&remove, "remove", false, "Unload and delete the weekly job")
	cmd.Flags().BoolVar(&noLoad, "no-load", false, "Write the plist without loading it")
	cmd.Flags().StringVar(&agentsDir, "agents-dir", "", "LaunchAgents directory (default ~/Library/LaunchAgents)")
	_ = cmd.Flags().MarkHidden("agents-dir")
	return cmd
}

func runSchedule(out io.Writer, agentsDir string, remove, load bool) error {
	exe, err := executable()
	if err != nil {
		exe = "dendrite"
	}
	if goos != "darwin" && agentsDir == "" {
		fmt.Fprintf(out, "[i] launchd is not available here; add to crontab:\n    30 9 * * 1 %s memory check --quiet\n", exe)
		return nil
	}
	dir, err := launchd.AgentsDir(agentsDir, os.UserHomeDir)
	if err != nil {
		return err
	}
	job := memoryJob
	job.Binary = exe
	if remove {
		return removeJob(out, job, dir, load)
	}
	return installJob(out, job, dir, load)
}

func removeJob(out io.Writer, job launchd.Job, dir string, load bool) error {
	path := job.Path(dir)
	if load {
		launchd.Unload(launchctl, path)
	}
	if _, err := job.Remove(dir); err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] Weekly memory check removed (%s)\n", path)
	return nil
}

func installJob(out io.Writer, job launchd.Job, dir string, load bool) error {
	path, err := job.Install(dir, launchd.LogDir(projectid.HomeDir))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[✓] Weekly memory check written: %s (Mondays 09:30)\n", path)
	if !load {
		return nil
	}
	if err := launchd.Load(launchctl, path); err != nil {
		return err
	}
	fmt.Fprintln(out, "[✓] Loaded into launchd")
	return nil
}
