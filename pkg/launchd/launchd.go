// Package launchd writes and loads the weekly LaunchAgents Dendrite schedules (the model catalog refresh, the
// memory check). A Job is plain data; installing it writes a plist and loading it goes through a Runner, so
// tests never reach the real launchd.
package launchd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Job is a weekly job: Binary runs Args on Weekday (0 Sunday .. 6) at Hour:Minute. Its output goes to
// <logDir>/<LogName>.log and .err. launchd also runs a job missed while the machine slept when it wakes.
type Job struct {
	Label, Binary, LogName string
	Args                   []string
	Weekday, Hour, Minute  int
}

// Plist returns the launchd property list of the job.
func (j Job) Plist(logDir string) string {
	var args strings.Builder
	for _, a := range append([]string{j.Binary}, j.Args...) {
		fmt.Fprintf(&args, "        <string>%s</string>\n", a)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
%s    </array>
    <key>StartCalendarInterval</key>
    <dict>
        <key>Weekday</key>
        <integer>%d</integer>
        <key>Hour</key>
        <integer>%d</integer>
        <key>Minute</key>
        <integer>%d</integer>
    </dict>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, j.Label, args.String(), j.Weekday, j.Hour, j.Minute,
		filepath.Join(logDir, j.LogName+".log"), filepath.Join(logDir, j.LogName+".err"))
}

// AgentsDir is dir, or ~/Library/LaunchAgents (the user's home comes from home) when dir is empty.
func AgentsDir(dir string, home func() (string, error)) (string, error) {
	if dir != "" {
		return dir, nil
	}
	h, err := home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "Library", "LaunchAgents"), nil
}

// Path is where the job's plist lives under the LaunchAgents directory dir.
func (j Job) Path(dir string) string { return filepath.Join(dir, j.Label+".plist") }

// Install writes the plist into dir and returns its path; loading it is the caller's step.
func (j Job) Install(dir, logDir string) (string, error) {
	path := j.Path(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return path, err
	}
	return path, os.WriteFile(path, []byte(j.Plist(logDir)), 0o644)
}

// Remove deletes the plist; a missing one is fine.
func (j Job) Remove(dir string) (string, error) {
	path := j.Path(dir)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return path, err
	}
	return path, nil
}

// Runner runs launchctl with args and returns its combined output.
type Runner func(args ...string) ([]byte, error)

// Exec is the Runner that runs the real launchctl.
func Exec(args ...string) ([]byte, error) { return exec.Command("launchctl", args...).CombinedOutput() }

// Domain is the launchd domain of the current user.
func Domain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

// Load (re)loads the plist at path: it boots out a loaded copy first, so a changed job takes effect.
func Load(run Runner, path string) error {
	_, _ = run("bootout", Domain(), path)
	if msg, err := run("bootstrap", Domain(), path); err != nil {
		return fmt.Errorf("launchctl bootstrap: %s", strings.TrimSpace(string(msg)))
	}
	return nil
}

// Unload boots the plist at path out of launchd; a job that is not loaded is fine.
func Unload(run Runner, path string) { _, _ = run("bootout", Domain(), path) }

// LogDir is where a job's logs go: <dendriteHome>/logs (created), or the temp directory when the Dendrite home
// is unknown.
func LogDir(dendriteHome func() (string, error)) string {
	home, err := dendriteHome()
	if err != nil {
		return os.TempDir()
	}
	dir := filepath.Join(home, "logs")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}
