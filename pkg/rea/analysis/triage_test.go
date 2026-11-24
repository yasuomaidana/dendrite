package analysis

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/testkit"
)

// triageRig is a rig whose service also routes the functions of the image, which the stand-in sidecar builds from
// the text of the binary (see binFunctions).
func triageRig(t *testing.T, content string) *rig {
	t.Helper()
	r := newRig(t)
	r.Functions = r.Inspector.(*CoreInspector).Client
	r.binary = writeBinary(t, content)
	return r
}

func tiersOf(t *testing.T, r *rig, id string) map[string]string {
	t.Helper()
	routed, err := r.Routing(id)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range routed {
		out[f.Name] = f.Tier
	}
	return out
}

// AC-1 of DEN-1063: cyclomatic complexity up to 15 goes to the local model.
func TestFunctionsUpToTheLimitGoToTheLocalModel(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 easy image")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-easy"})
	if err != nil || res.State.Stage != StageDone || res.State.Gate != "" {
		t.Fatalf("an image with no hard function runs through: %+v, %v", res, err)
	}
	if got := tiersOf(t, r, "tri-easy"); got["main"] != TierLocal || got["helper"] != TierLocal {
		t.Errorf("main (5) and helper (15) are local, got %v", got)
	}
	if tr := res.State.Triage; tr == nil || tr.Local != 2 || tr.Paid != 0 || tr.Limit != CyclomaticLimit {
		t.Errorf("summary in the checkpoint: %+v", res.State.Triage)
	}
}

// AC-2: harder functions stop at a pause gate. AC-3: the tier chosen for each function is recorded in the checkpoint.
func TestHardFunctionsStopAtThePauseGate(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 HARD image")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-hard"})
	if !errors.Is(err, ErrPaused) || res.State.Stage != StageTriaged || !res.State.Paused || !strings.Contains(res.State.Gate, "1 function(s)") {
		t.Fatalf("the gate: %+v, %v", res, err)
	}
	if got := tiersOf(t, r, "tri-hard"); got["crypt_loop"] != TierPaid || got["main"] != TierLocal {
		t.Errorf("routing recorded at the gate: %v", got)
	}
}

// The gate is part of the checkpoint on disk, and resuming on the local tier does not pass it.
func TestTheGateSurvivesInTheCheckpointAndALocalResume(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 HARD image")
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-keep"}); !errors.Is(err, ErrPaused) {
		t.Fatal(err)
	}
	if st, _ := r.Status("tri-keep"); st.Gate == "" || st.Triage == nil || st.Triage.Paid != 1 {
		t.Errorf("the checkpoint on disk keeps the gate and the summary: %+v", st)
	}
	again, err := r.Resume(ctx(), "tri-keep", Options{})
	if !errors.Is(err, ErrPaused) || again.State.Gate == "" {
		t.Errorf("resuming on the local tier does not pass the gate: %+v, %v", again, err)
	}
}

// The paid model is only allowed by a resume on the paid tier.
func TestResumingOnThePaidTierPassesTheGate(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 HARD image")
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-go"}); !errors.Is(err, ErrPaused) {
		t.Fatal(err)
	}
	done, err := r.Resume(ctx(), "tri-go", Options{Tier: "paid"})
	if err != nil || done.State.Stage != StageDone || done.State.Gate != "" || done.State.Tier != "paid" {
		t.Fatalf("resume on the paid tier: %+v, %v", done, err)
	}
	doc, err := os.ReadFile(filepath.Join(r.Store.WorkspaceDir("tri-go"), "triage.md"))
	if err != nil || !strings.Contains(string(doc), "crypt_loop: cyclomatic 22") {
		t.Errorf("the triage document is part of the searchable workspace: %v\n%s", err, doc)
	}
}

// A count the sidecar could not finish is a lower bound: the function goes to the paid tier whatever its value.
func TestAPartialCountIsNeverTrustedToTheLocalModel(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 PARTIAL image")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-part"})
	if !errors.Is(err, ErrPaused) || res.State.Triage.Paid != 1 {
		t.Fatalf("one partial function: %+v, %v", res, err)
	}
	if got := tiersOf(t, r, "tri-part"); got["walked_out"] != TierPaid || got["main"] != TierLocal {
		t.Errorf("routing: %v", got)
	}
}

func TestAnAnalysisStartedOnThePaidTierIsNotGated(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 HARD image")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-paid", Tier: "paid"})
	if err != nil || res.State.Stage != StageDone || res.State.Triage.Paid != 1 {
		t.Fatalf("paid from the start: %+v, %v", res, err)
	}
}

func TestEveryPageOfFunctionsIsRouted(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 MANY image")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-many"})
	if err != nil || res.State.Triage.Local != 102 {
		t.Fatalf("102 functions over three pages: %+v, %v", res.State.Triage, err)
	}
}

func TestATruncatedFunctionListStopsAtTheGate(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 BUDGET image")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-cut"})
	if !errors.Is(err, ErrPaused) || !strings.Contains(res.State.Gate, "undercounted") || !res.State.Triage.Truncated {
		t.Fatalf("a spent budget cannot be trusted: %+v, %v", res, err)
	}
}

func TestATargetWhoseFunctionsCannotBeListedIsNotTriaged(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 easy image")
	r.Functions = listerFunc(func(context.Context, string) (*coreclient.FunctionList, error) {
		return nil, coreclient.ErrNoFunctions
	})
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-none"})
	if err != nil || res.State.Stage != StageDone || res.State.Triage != nil || res.State.Gate != "" {
		t.Fatalf("not triaged, not gated: %+v, %v", res, err)
	}
	if routed, err := r.Routing("tri-none"); err != nil || len(routed) != 0 {
		t.Errorf("nothing routed: %v, %v", routed, err)
	}
}

func TestAFailedFunctionListIsRecordedAndResumeRetriesIt(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 easy image")
	boom := errors.New("sidecar broke")
	r.Functions = listerFunc(func(context.Context, string) (*coreclient.FunctionList, error) { return nil, boom })
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-fail"})
	if !errors.Is(err, boom) || res.State.Stage != StageWritten || res.State.Error == "" {
		t.Fatalf("a failing triage stays at the written stage: %+v, %v", res, err)
	}
	r.Functions = r.Inspector.(*CoreInspector).Client
	if done, err := r.Resume(ctx(), "tri-fail", Options{}); err != nil || done.State.Triage == nil {
		t.Errorf("resume retries triage: %+v, %v", done, err)
	}
}

// blockWithDirectory puts a directory with a file in it where a file of the analysis goes, so writing it fails.
func blockWithDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTriageReportsWhatItCannotWrite(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 easy image")
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-io", Tier: "paid"}); err != nil {
		t.Fatal(err)
	}
	st, _ := r.Status("tri-io")
	st.Stage = StageWritten
	for _, name := range []string{"triage.json", "workspace/triage.md"} {
		path := filepath.Join(r.Store.Dir("tri-io"), name)
		blockWithDirectory(t, path)
		if err := r.triage(ctx(), &st); err == nil {
			t.Errorf("%s cannot be written over a directory", name)
		}
		_ = os.RemoveAll(path)
	}
}

func TestRoutingReportsADamagedTriageFile(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 easy image")
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-bad"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.triagePath("tri-bad"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Routing("tri-bad"); err == nil {
		t.Error("a damaged triage.json is an error")
	}
	blockWithDirectory(t, r.triagePath("tri-bad"))
	if _, err := r.Routing("tri-bad"); err == nil {
		t.Error("an unreadable triage.json is an error")
	}
}

func TestTheTriageDocumentListsAtMostTheFirstHardFunctions(t *testing.T) {
	d := triageDoc{Triage: Triage{Limit: 15}}
	for i := 0; i < maxHardListed+5; i++ {
		d.Functions = append(d.Functions, Routed{Start: "0x1", Cyclomatic: 20, Tier: TierPaid})
	}
	text := triageText(d)
	if strings.Count(text, "cyclomatic 20") != maxHardListed || !strings.Contains(text, "more functions above the limit") || !strings.Contains(text, "(unnamed)") {
		t.Errorf("document:\n%s", text)
	}
}

type listerFunc func(context.Context, string) (*coreclient.FunctionList, error)

func (f listerFunc) Functions(ctx context.Context, path string) (*coreclient.FunctionList, error) {
	return f(ctx, path)
}

// elfTriageRig is a rig whose core sidecar cannot list the functions of the target (an ELF), with a rizin stand-in
// that can.
func elfTriageRig(t *testing.T, images ...string) *rig {
	t.Helper()
	r := engineRig(t, "\x7fELF", images...)
	r.Functions = listerFunc(func(context.Context, string) (*coreclient.FunctionList, error) {
		return nil, coreclient.ErrFunctionsUnsupported
	})
	return r
}

func TestEngineFunctionsAreRoutedWhenTheSidecarCannotListThem(t *testing.T) {
	r := elfTriageRig(t, "dendrite-rea/rizin:1")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-eng"})
	if err != nil || res.State.Stage != StageDone || res.State.Triage == nil || res.State.Gate != "" {
		t.Fatalf("%+v, %v", res, err)
	}
	wantTriage(t, res.State.Triage, "engine", 3, 0, 0)
	if got := tiersOf(t, r, "tri-eng"); got["main"] != TierLocal || got["helper"] != TierLocal {
		t.Errorf("routing: %v", got)
	}
}

func TestAHardEngineFunctionStopsAtTheGateUntilThePaidTier(t *testing.T) {
	r := elfTriageRig(t, "dendrite-rea/rizin:1")
	t.Setenv("STANDIN_MODE", "r2hard")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-hard"})
	if !errors.Is(err, ErrPaused) || res.State.Triage.Paid != 1 || !strings.Contains(res.State.Gate, "need a paid model") {
		t.Fatalf("%+v, %v", res, err)
	}
	if got := tiersOf(t, r, "tri-hard"); got["main"] != TierPaid || got["helper"] != TierLocal {
		t.Errorf("routing: %v", got)
	}
	done, err := r.Resume(ctx(), "tri-hard", Options{Tier: "paid"})
	if err != nil || done.State.Stage != StageDone || done.State.Gate != "" {
		t.Errorf("the paid tier clears the gate: %+v, %v", done.State, err)
	}
}

func TestEngineFunctionsWithoutComplexityAreCountedNotRouted(t *testing.T) {
	r := elfTriageRig(t, "dendrite-rea/objdump:1")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-skip", Engine: "objdump"})
	if err != nil || res.State.Triage == nil || res.State.Gate != "" {
		t.Fatalf("%+v, %v", res, err)
	}
	wantTriage(t, res.State.Triage, "engine", 0, 0, 2)
	if got := tiersOf(t, r, "tri-skip"); len(got) != 0 {
		t.Errorf("nothing is routed: %v", got)
	}
	text, _ := os.ReadFile(filepath.Join(r.Store.WorkspaceDir("tri-skip"), "triage.md"))
	testkit.Contains(t, string(text), "2 functions have no complexity", "Functions listed by: engine")
}

func TestEngineOnlyTriageNeedsNoCoreLister(t *testing.T) {
	r := engineRig(t, "\x7fELF", "dendrite-rea/rizin:1") // Functions stays nil
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-only"})
	if err != nil || res.State.Triage == nil || res.State.Triage.Source != "engine" {
		t.Errorf("%+v, %v", res.State, err)
	}
}

func TestATargetWithNeitherCoreNorEngineFunctionsIsNotTriaged(t *testing.T) {
	r := elfTriageRig(t) // no image: no engine runs
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-nofn"})
	if err != nil || res.State.Stage != StageDone || res.State.Triage != nil {
		t.Errorf("%+v, %v", res.State, err)
	}
}

func TestTheCoreListIsStillTheSourceOfAPETarget(t *testing.T) {
	r := triageRig(t, "MZ\x90\x00 easy image")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-core"})
	if err != nil || res.State.Triage == nil || res.State.Triage.Source != "core" {
		t.Errorf("%+v, %v", res.State, err)
	}
}

func TestAnEngineResultThatCannotBeReadStopsTheTriage(t *testing.T) {
	r := elfTriageRig(t, "dendrite-rea/rizin:1")
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "tri-bad"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.enginePath("tri-bad"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, _ := r.Store.Load("tri-bad")
	st.Stage = StageEngine // back to the stage triage follows
	if err := r.Store.Save(&st); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resume(ctx(), "tri-bad", Options{}); err == nil {
		t.Error("a corrupt engine.json is an error")
	}
}

// wantTriage checks the summary of a routing: who listed the functions, how many went to each tier and how many were skipped.
func wantTriage(t *testing.T, tr *Triage, source string, local, paid, skipped int) {
	t.Helper()
	if tr.Source != source || tr.Local != local || tr.Paid != paid || tr.Skipped != skipped || tr.Limit != CyclomaticLimit {
		t.Errorf("triage %+v, want %s %d local %d paid %d skipped", tr, source, local, paid, skipped)
	}
}
