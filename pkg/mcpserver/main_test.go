package mcpserver

import (
	"fmt"
	"os"
	"testing"

	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/rag/ragtest"
)

// TestMain serves retrieval from the dendrite-rag stand-in, never from a sidecar that
// happens to be installed on the machine, and Plane from the real dendrite-plane build.
func TestMain(m *testing.M) {
	cleanRag, err := ragtest.Install()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cleanPlane, err := planetest.Install()
	if err != nil {
		cleanRag()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	cleanPlane()
	cleanRag()
	os.Exit(code)
}
