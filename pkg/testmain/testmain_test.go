package testmain

import (
	"errors"
	"testing"
)

type codeRunner struct {
	code int
	ran  bool
}

func (f *codeRunner) Run() int { f.ran = true; return f.code }

func TestRun(t *testing.T) {
	cases := []struct {
		name        string
		installErr  error
		testsCode   int
		wantExit    int
		wantRan     bool
		wantCleanup bool
	}{
		{"passing tests exit 0 after cleanup", nil, 0, 0, true, true},
		{"failing tests keep their exit code", nil, 3, 3, true, true},
		{"a failed install exits 1 and runs nothing", errors.New("no sidecar"), 0, 1, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := -1
			old := exit
			exit = func(code int) { got = code }
			defer func() { exit = old }()

			cleaned := false
			r := &codeRunner{code: c.testsCode}
			Run(r, func() (func(), error) {
				if c.installErr != nil {
					return nil, c.installErr
				}
				return func() { cleaned = true }, nil
			})
			if got != c.wantExit || r.ran != c.wantRan || cleaned != c.wantCleanup {
				t.Fatalf("exit=%d ran=%v cleaned=%v, want exit=%d ran=%v cleaned=%v", got, r.ran, cleaned, c.wantExit, c.wantRan, c.wantCleanup)
			}
		})
	}
}
