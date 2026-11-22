package modelcatalog

import (
	"os"

	"Dendrite/pkg/launchd"
)

// ScheduleLabel is the launchd label of the weekly refresh.
const ScheduleLabel = "dev.dendrite.models"

// scheduleJob is the weekly refresh: `dendrite cleft models refresh` every Monday at 09:00.
var scheduleJob = launchd.Job{
	Label: ScheduleLabel, LogName: "models-refresh", Weekday: 1, Hour: 9,
	Args: []string{"cleft", "models", "refresh", "--quiet"},
}

// LaunchdPlist returns the plist that runs `dendrite cleft models refresh` every Monday at 09:00.
// launchd also runs a job missed while the machine slept the next time it wakes.
func LaunchdPlist(binary, logDir string) string {
	j := scheduleJob
	j.Binary = binary
	return j.Plist(logDir)
}

// userHomeDir is the user's home directory; tests replace it.
var userHomeDir = os.UserHomeDir

// PlistPath is where the weekly refresh's plist lives for the user (under launchAgentsDir, which
// defaults to ~/Library/LaunchAgents).
func PlistPath(launchAgentsDir string) (string, error) {
	dir, err := launchd.AgentsDir(launchAgentsDir, userHomeDir)
	return scheduleJob.Path(dir), err
}

// InstallSchedule writes the plist and returns its path; loading it is the caller's step.
func InstallSchedule(launchAgentsDir, binary, logDir string) (string, error) {
	dir, err := launchd.AgentsDir(launchAgentsDir, userHomeDir)
	if err != nil {
		return "", err
	}
	j := scheduleJob
	j.Binary = binary
	return j.Install(dir, logDir)
}

// RemoveSchedule deletes the plist; a missing one is fine.
func RemoveSchedule(launchAgentsDir string) (string, error) {
	dir, err := launchd.AgentsDir(launchAgentsDir, userHomeDir)
	if err != nil {
		return "", err
	}
	return scheduleJob.Remove(dir)
}
