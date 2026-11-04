package cli

import (
	"io"
	"os"
)

// isTerminal reports whether in is the process's interactive stdin; a seam for tests.
var IsTerminal = func(in io.Reader) bool {
	f, ok := in.(*os.File)
	return ok && f == os.Stdin && StdinIsTerminal()
}

func Short(sha string) string {
	if sha == "" {
		return "-"
	}
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// stdinIsTerminal reports whether stdin is an interactive terminal. /dev/null is a character
// device too, but nobody can answer a prompt there (CI, installer scripts, `< /dev/null`).
func StdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(fi, null) {
		return false
	}
	return true
}
