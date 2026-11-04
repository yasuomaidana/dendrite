package main

import "errors"

const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
	// ExitHold and ExitQuarantine are the verdicts of `dendrite immune scan` that are not a pass.
	ExitHold       = 3
	ExitQuarantine = 4
)

// verdictError carries a gate verdict's exit code out of a command.
type verdictError struct {
	code int
	msg  string
}

func (e *verdictError) Error() string { return e.msg }

// exitCodeFor maps a command error to the process exit code: usage mistakes exit 2, runtime failures exit 1.
func exitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	var ve *verdictError
	if errors.As(err, &ve) {
		return ve.code
	}
	if isUsageError(err) {
		return ExitUsage
	}
	return ExitError
}
