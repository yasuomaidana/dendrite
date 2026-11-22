package export

import (
	"testing"

	"Dendrite/pkg/coreclient/coretest"
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/testmain"
)

// The export tests run a real analysis against the stand-in sidecars (dendrite-core, dendrite-rag) and the real
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
