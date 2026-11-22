package analysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/coreclient/coretest"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/testmain"
)

// The analysis talks to the dendrite-core and dendrite-rag sidecars; the tests use their stand-in commands, so an
// installed sidecar is never used.
func TestMain(m *testing.M) {
	testmain.Run(m, func() (func(), error) {
		core, err := coretest.Install()
		if err != nil {
			return nil, err
		}
		ragStandin, err := ragtest.Install()
		if err != nil {
			core()
			return nil, err
		}
		return func() { ragStandin(); core() }, nil
	})
}

// rig is a service over a temporary store, the stand-in sidecars and one binary on disk.
type rig struct {
	*Service
	t      *testing.T
	binary string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	core := coreclient.New(sidecar.DefaultLocator())
	t.Cleanup(func() { _ = core.Close(); rag.CloseSidecars() })
	return &rig{
		Service: &Service{Store: &Store{Root: filepath.Join(t.TempDir(), "rea")}, Inspector: &CoreInspector{Client: core}, Index: NewRagIndex()},
		t:       t,
		binary:  writeBinary(t, "MZ\x90\x00 pe image"),
	}
}

// writeBinary writes a file with the content and returns its path.
func writeBinary(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.exe")
	testkit.WriteFile(t, path, content, 0o600)
	return path
}

// canWrite reports whether permission bits stop writes in dir (they do not for root).
func canWrite(dir string) bool {
	probe := filepath.Join(dir, ".probe")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		return false
	}
	_ = os.Remove(probe)
	return true
}

// readOnly makes dir read-only until the test ends and reports whether that stops writes.
func readOnly(t *testing.T, dir string) bool {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	return !canWrite(dir)
}

func ctx() context.Context { return context.Background() }
