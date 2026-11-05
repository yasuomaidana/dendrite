package cli

import "errors"

const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
	// ExitHold and ExitQuarantine are the verdicts of `dendrite immune scan` that are not a pass.
	ExitHold       = 3
	ExitQuarantine = 4
)

// VerdictError carries a gate verdict's exit code out of a command.
type VerdictError struct {
	Code int
	Msg  string
}

func (e *VerdictError) Error() string { return e.Msg }

// ExitCodeFor maps a command error to the process exit code: a gate verdict exits with its own code, usage
// mistakes exit 2, runtime failures exit 1.
func ExitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	var ve *VerdictError
	if errors.As(err, &ve) {
		return ve.Code
	}
	if IsUsageError(err) {
		return ExitUsage
	}
	return ExitError
}
