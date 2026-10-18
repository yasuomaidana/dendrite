package main

const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

// exitCodeFor maps a command error to the process exit code: usage mistakes exit 2, runtime failures exit 1.
func exitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	if isUsageError(err) {
		return ExitUsage
	}
	return ExitError
}
