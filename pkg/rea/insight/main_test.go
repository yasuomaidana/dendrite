package insight

import (
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/coreclient/coretest"
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/testmain"
)

// The insight tests run a real analysis against the stand-in sidecars (dendrite-core, dendrite-rag) and the real
// dendrite-plane sidecar over a test board, so an installed sidecar is never used.
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
		plane, err := planetest.Install()
		if err != nil {
			ragStandin()
			core()
			return nil, err
		}
		return func() { plane(); ragStandin(); core() }, nil
	})
}

// finishedAnalysis runs a real analysis of a file named file with the content and returns the service and its id.
func finishedAnalysis(t *testing.T, file, content string) (*analysis.Service, string) {
	t.Helper()
	return analysisWith(t, file, content, nil)
}

// analysisWith is finishedAnalysis with an engine runner when engines is not nil.
func analysisWith(t *testing.T, file, content string, engines analysis.EngineRunner) (*analysis.Service, string) {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	core := coreclient.New(sidecar.DefaultLocator())
	t.Cleanup(func() { _ = core.Close(); rag.CloseSidecars() })
	svc := &analysis.Service{Store: &analysis.Store{Root: filepath.Join(t.TempDir(), "rea")}, Inspector: &analysis.CoreInspector{Client: core}, Index: analysis.NewRagIndex(), Engines: engines}
	path := filepath.Join(t.TempDir(), file)
	testkit.WriteFile(t, path, content, 0o600)
	if _, err := svc.Analyze(ctx(), path, analysis.Options{ID: "an-1"}); err != nil {
		t.Fatal(err)
	}
	return svc, "an-1"
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	testkit.WriteFile(t, filepath.Join(dir, name), content, 0o644)
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }
