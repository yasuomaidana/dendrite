package export

import (
	"testing"

	"Dendrite/pkg/coreclient/coretest"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/testmain"
)

// The export tests run a real analysis against the stand-in sidecars (dendrite-core, dendrite-rag) and the real
// dendrite-plane sidecar over a test board, so an installed sidecar is never used.
func TestMain(m *testing.M) {
	testmain.Run(m, func() (func(), error) {
		var cleanups []func()
		undo := func() {
			for i := len(cleanups) - 1; i >= 0; i-- {
				cleanups[i]()
			}
		}
		for _, install := range []func() (func(), error){coretest.Install, ragtest.Install} {
			cleanup, err := install()
			if err != nil {
				undo()
				return nil, err
			}
			cleanups = append(cleanups, cleanup)
		}
		return undo, nil
	})
}
