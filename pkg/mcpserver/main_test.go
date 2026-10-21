package mcpserver

import (
	"os"
	"testing"

	"Dendrite/pkg/rag"
)

// TestMain keeps retrieval in-process: the tests must not depend on a dendrite-rag
// sidecar that happens to be installed on the machine.
func TestMain(m *testing.M) {
	_ = os.Setenv(rag.SidecarEnv, "off")
	os.Exit(m.Run())
}
