package mcpserver

import (
	"testing"

	"Dendrite/pkg/rag/ragtest"
)

// TestMain serves retrieval from the dendrite-rag stand-in, never from a sidecar that
// happens to be installed on the machine.
func TestMain(m *testing.M) { ragtest.Main(m) }
