package session

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"Dendrite/pkg/engram"
	"Dendrite/pkg/testkit"
)

func TestSessionResumePrintsTheEngramSnapshot(t *testing.T) {
	inRepoOnBranch(t, "agent/den-1")
	stand := filepath.Join(t.TempDir(), "engram")
	testkit.WriteFile(t, stand, "#!/bin/sh\necho '## Memory from Previous Sessions'\necho '- [manual] **Chose Engram**'\n", 0o755)
	t.Setenv(engram.BinaryEnv, stand)

	var buf bytes.Buffer
	testkit.Check(t, RunSessionCommand(context.Background(), []string{"checkpoint"}, &buf))
	buf.Reset()
	testkit.Check(t, RunSessionCommand(context.Background(), []string{"resume"}, &buf))
	testkit.Contains(t, buf.String(), "## Memory (Engram)", "Chose Engram")

	empty := filepath.Join(t.TempDir(), "engram")
	testkit.WriteFile(t, empty, "#!/bin/sh\necho 'No previous session memories found.'\n", 0o755)
	t.Setenv(engram.BinaryEnv, empty)
	buf.Reset()
	testkit.Check(t, RunSessionCommand(context.Background(), []string{"resume"}, &buf))
	testkit.Contains(t, buf.String(), "(no memories yet)", "dendrite memory save")
}
