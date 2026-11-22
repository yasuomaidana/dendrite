//go:build e2e

package analysis

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/coreclient/coretest"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/sidecar"
)

// pe64Fixture is a real 64-bit Windows executable (MinGW-w64 hello world) that ships with the Go toolchain.
func pe64Fixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(runtime.GOROOT(), "src", "debug", "pe", "testdata", "gcc-amd64-mingw-exec")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the PE64 fixture is missing from the Go toolchain: %v", err)
	}
	return path
}

// TestAnalyzeRealPE64 is the proof of AC-1 of DEN-1062: a real PE64 image is parsed by the dendrite-core sidecar of
// DCORE-31 and ends as a bundle under the 4,000-token boundary. It needs a dendrite-core built with pe.parse:
//
//	DENDRITE_CORE_REAL_BIN=$HOME/.dendrite/bin/dendrite-core go test -tags e2e ./pkg/rea/analysis -run RealPE64
func TestAnalyzeRealPE64(t *testing.T) {
	bin := os.Getenv("DENDRITE_CORE_REAL_BIN")
	if bin == "" {
		t.Fatal("set DENDRITE_CORE_REAL_BIN to a dendrite-core binary with pe.parse")
	}
	t.Setenv(sidecar.EnvVar(coretest.BinaryName), bin)
	t.Setenv("DENDRITE_HOME", t.TempDir())

	core := coreclient.New(sidecar.DefaultLocator())
	t.Cleanup(func() { _ = core.Close(); rag.CloseSidecars() })
	info, err := core.PEParse(ctx(), pe64Fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if info.Machine == "" || !info.PE32Plus || !strings.HasPrefix(info.ImageBase, "0x") {
		t.Errorf("not read as a PE64 image: %+v", info)
	}
	if !hasSection(info, ".text") || len(info.Imports) == 0 {
		t.Errorf("a MinGW executable has a .text section and imports, got %d sections and %d imports", len(info.Sections), len(info.Imports))
	}

	svc := newRig(t)
	svc.Parser = core
	got, err := svc.Analyze(ctx(), pe64Fixture(t), Options{ID: "pe64-real"})
	if err != nil {
		t.Fatal(err)
	}
	if got.State.Stage != StageDone || got.Bundle == nil {
		t.Fatalf("analysis did not finish: %+v", got.State)
	}
	if got.Bundle.TokenEstimate >= 4000 {
		t.Errorf("bundle is %d tokens, want under 4,000", got.Bundle.TokenEstimate)
	}
	if len(got.Bundle.CrossReferences) == 0 {
		t.Error("the imported DLLs are missing from the bundle")
	}
}

func hasSection(info *coreclient.PEInfo, name string) bool {
	for _, s := range info.Sections {
		if s.Name == name {
			return true
		}
	}
	return false
}
