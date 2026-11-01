// Package testmain runs a test binary's TestMain with a setup step that can fail and a cleanup.
package testmain

import (
	"fmt"
	"os"
)

// Runner is what TestMain runs; *testing.M satisfies it.
type Runner interface{ Run() int }

// exit is os.Exit, replaced by tests.
var exit = os.Exit

// Run calls install, runs the tests, calls the cleanup install returned and exits with the tests'
// code. When install fails it prints the error and exits 1 without running anything.
func Run(m Runner, install func() (func(), error)) {
	cleanup, err := install()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		exit(1)
		return
	}
	code := m.Run()
	cleanup()
	exit(code)
}
