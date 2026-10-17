package main

import (
	"errors"

	"Dendrite/pkg/cli"
)

const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

func exitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, cli.ErrUnknownCommand) {
		return ExitUsage
	}
	return ExitError
}
