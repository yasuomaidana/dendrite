//go:build e2e

package analysis

import (
	"testing"

	"Dendrite/pkg/coreclient"
)

// TestTriageRealPE64 routes the functions of a real PE64 image listed by the dendrite-core of DCORE-33. It needs a
// dendrite-core built with bin.functions:
//
//	DENDRITE_CORE_REAL_BIN=<core-rs>/target/release/dendrite-core go test -tags e2e ./pkg/rea/analysis -run TriageRealPE64
func TestTriageRealPE64(t *testing.T) {
	core := realCore(t)
	svc := newRig(t)
	svc.Inspector = &CoreInspector{Client: core}
	svc.Functions = core
	// Run on the paid tier so a hard function in the fixture does not stop the analysis at the gate.
	got, err := svc.Analyze(ctx(), pe64Fixture(t), Options{ID: "tri-real", Tier: TierPaid})
	if err != nil || got.State.Stage != StageDone {
		t.Fatalf("analysis did not finish: %v %+v", err, got)
	}
	tr := got.State.Triage
	if tr == nil || tr.Local+tr.Paid == 0 {
		t.Fatalf("a MinGW executable has functions, got triage %+v", tr)
	}
	routed, err := svc.Routing("tri-real")
	if err != nil || len(routed) != tr.Local+tr.Paid {
		t.Fatalf("every function is routed: %d of %d, %v", len(routed), tr.Local+tr.Paid, err)
	}
	for _, f := range routed {
		if want := routeTier(coreclient.Function{Cyclomatic: f.Cyclomatic}); f.Tier != want {
			t.Errorf("function %s (cyclomatic %d) is on tier %s, want %s", f.Start, f.Cyclomatic, f.Tier, want)
		}
	}
	t.Logf("%d functions: %d local, %d paid (truncated: %t)", tr.Local+tr.Paid, tr.Local, tr.Paid, tr.Truncated)
}
