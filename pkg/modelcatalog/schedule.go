package modelcatalog

import (
	"fmt"
	"os"
	"path/filepath"
)

// ScheduleLabel is the launchd label of the weekly refresh.
const ScheduleLabel = "dev.dendrite.models"

// LaunchdPlist returns the plist that runs `dendrite cleft models refresh` every Monday at 09:00.
// launchd also runs a job missed while the machine slept the next time it wakes.
func LaunchdPlist(binary, logDir string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>cleft</string>
        <string>models</string>
        <string>refresh</string>
        <string>--quiet</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict>
        <key>Weekday</key>
        <integer>1</integer>
        <key>Hour</key>
        <integer>9</integer>
        <key>Minute</key>
        <integer>0</integer>
    </dict>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, ScheduleLabel, binary, filepath.Join(logDir, "models-refresh.log"), filepath.Join(logDir, "models-refresh.err"))
}

// userHomeDir is the user's home directory; tests replace it.
var userHomeDir = os.UserHomeDir

// PlistPath is where the weekly refresh's plist lives for the user (under launchAgentsDir, which
// defaults to ~/Library/LaunchAgents).
func PlistPath(launchAgentsDir string) (string, error) {
	if launchAgentsDir == "" {
		home, err := userHomeDir()
		if err != nil {
			return "", err
		}
		launchAgentsDir = filepath.Join(home, "Library", "LaunchAgents")
	}
	return filepath.Join(launchAgentsDir, ScheduleLabel+".plist"), nil
}

// InstallSchedule writes the plist and returns its path; loading it is the caller's step.
func InstallSchedule(launchAgentsDir, binary, logDir string) (string, error) {
	path, err := PlistPath(launchAgentsDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(LaunchdPlist(binary, logDir)), 0o644)
}

// RemoveSchedule deletes the plist; a missing one is fine.
func RemoveSchedule(launchAgentsDir string) (string, error) {
	path, err := PlistPath(launchAgentsDir)
	if err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return path, err
	}
	return path, nil
}
