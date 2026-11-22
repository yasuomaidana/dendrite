package analysis

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/directive"
	"Dendrite/pkg/rea"
	"Dendrite/pkg/rea/engine"
	"Dendrite/pkg/rea/engine/enginetest"
)

// engineRig is a rig whose analyses also run an engine, over stand-ins for docker and the tools, with the given
// images "built".
func engineRig(t *testing.T, content string, images ...string) *rig {
	t.Helper()
	r := newRig(t)
	enginetest.UsePath(t, images...)
	r.Engines = engine.NewService()
	r.binary = writeBinary(t, content)
	return r
}

func TestTheEngineStageListsFunctionsAndWritesTheirDocuments(t *testing.T) {
	r := engineRig(t, "\x7fELF", "dendrite-rea/rizin:1")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "eng-1"})
	if err != nil || res.State.Stage != StageDone || res.State.EngineRan != "rizin" || res.State.EngineMode != "container" {
		t.Fatalf("%+v, %v", res, err)
	}
	dir := r.Store.WorkspaceDir("eng-1")
	for _, doc := range []string{"functions/0x401000-main.md", "functions/0x401040-helper.md"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(doc))); err != nil {
			t.Errorf("%s: %v", doc, err)
		}
	}
}

func TestTheEngineFunctionsAreSavedAndTheirDisassemblyIsSearchable(t *testing.T) {
	r := engineRig(t, "\x7fELF", "dendrite-rea/rizin:1")
	if _, err := r.Analyze(ctx(), r.binary, Options{ID: "eng-2"}); err != nil {
		t.Fatal(err)
	}
	fns, err := r.EngineFunctions("eng-2")
	if err != nil || len(fns) != 3 || fns[0].Name != "main" || !strings.Contains(fns[0].Disassembly, "push rbp") {
		t.Errorf("%+v, %v", fns, err)
	}
	hits, err := r.Search(ctx(), "eng-2", "push rbp", 3)
	if err != nil || !hasHit(hits, "functions/0x401000-main.md", "push rbp") {
		t.Errorf("the disassembly is searchable: %+v, %v", hits, err)
	}
}

func TestWithNoEngineUsableTheAnalysisGoesOnAndSaysWhy(t *testing.T) {
	r := engineRig(t, "\x7fELF") // Docker answers, no image is built
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "eng-none"})
	if err != nil || res.State.Stage != StageDone || res.State.EngineRan != "" || !strings.Contains(res.State.EngineNote, "dendrite retro engines build rizin") {
		t.Fatalf("%+v, %v", res.State, err)
	}
	if fns, err := r.EngineFunctions("eng-none"); err != nil || len(fns) != 0 {
		t.Errorf("%+v, %v", fns, err)
	}
	off, err := r.Analyze(ctx(), r.binary, Options{ID: "eng-off", Engine: "none"})
	if err != nil || !strings.Contains(off.State.EngineNote, "disabled") {
		t.Errorf("%+v, %v", off.State, err)
	}
}

func TestAnEngineThatFailsUnderAutoIsRecordedButWhenNamedIsAnError(t *testing.T) {
	r := engineRig(t, "\x7fELF", "dendrite-rea/rizin:1")
	t.Setenv("STANDIN_MODE", "r2broken")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "eng-auto"})
	if err != nil || res.State.Stage != StageDone || !strings.Contains(res.State.EngineNote, "went on without it") {
		t.Errorf("auto: %+v, %v", res.State, err)
	}
	res, err = r.Analyze(ctx(), r.binary, Options{ID: "eng-named", Engine: "rizin"})
	if err == nil || res.State.Stage != StageWritten || !strings.Contains(res.State.Error, "rizin (container)") {
		t.Errorf("named: %+v, %v", res.State, err)
	}
}

func TestARefusedPolicyStopsTheAnalysisAtTheEngineStage(t *testing.T) {
	r := engineRig(t, "\x7fELF", "dendrite-rea/rizin:1")
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "eng-host", Sandbox: "host"})
	if !errors.Is(err, rea.ErrUntrustedOnHost) || res.State.Stage != StageWritten {
		t.Errorf("an untrusted binary on the host: %+v, %v", res.State, err)
	}
	if _, err := r.Resume(ctx(), "eng-host", Options{Trusted: true}); err != nil {
		t.Errorf("--trusted lets the host run it: %v", err)
	}
	if st, _ := r.Store.Load("eng-host"); !st.Trusted || st.EngineMode != "host" || st.EngineRan != "rizin" {
		t.Errorf("trust is recorded: %+v", st)
	}
}

func TestResumeWithAnotherEngineRunsTheEngineStageAgain(t *testing.T) {
	r := engineRig(t, "\x7fELF", "dendrite-rea/rizin:1", "dendrite-rea/objdump:1")
	if res, err := r.Analyze(ctx(), r.binary, Options{ID: "eng-sw"}); err != nil || res.State.EngineRan != "rizin" {
		t.Fatalf("%+v, %v", res.State, err)
	}
	res, err := r.Resume(ctx(), "eng-sw", Options{Engine: "objdump"})
	if err != nil || res.State.Stage != StageDone || res.State.EngineRan != "objdump" || res.State.Engine != "objdump" {
		t.Fatalf("%+v, %v", res.State, err)
	}
	if fns, _ := r.EngineFunctions("eng-sw"); len(fns) != 2 {
		t.Errorf("the objdump functions replaced the rizin ones: %+v", fns)
	}
	if again, err := r.Resume(ctx(), "eng-sw", Options{}); err != nil || again.State.EngineRan != "objdump" {
		t.Errorf("resuming without a change keeps the engine: %+v, %v", again.State, err)
	}
}

func TestRetargetKeepsTheStageOfAnAnalysisThatNeverGotPastParsing(t *testing.T) {
	r := newRig(t)
	st := State{Stage: StageParsed}
	r.retarget(&st, Options{Engine: "rizin"})
	if st.Stage != StageParsed || st.Engine != "rizin" {
		t.Errorf("%+v", st)
	}
	back := State{Stage: StageDone, Gate: "waiting"}
	r.retarget(&back, Options{Sandbox: "host"})
	if back.Stage != StageWritten || back.Gate != "" || back.Sandbox != "host" {
		t.Errorf("an analysis past the written stage goes back to it: %+v", back)
	}
}

func TestEngineDefaultsComeFromTheEnvironmentThenTheDirective(t *testing.T) {
	rule := directive.Defaults().Rea
	rule.Engine = "radare2"
	if got := (Options{}).WithEngineDefaults(rule); got.Engine != "radare2" || got.Sandbox != "container" {
		t.Errorf("%+v", got)
	}
	t.Setenv(engine.EnvEngine, "none")
	if got := (Options{}).WithEngineDefaults(rule); got.Engine != "none" {
		t.Errorf("%+v", got)
	}
	if got := (Options{Engine: "objdump", Sandbox: "host"}).WithEngineDefaults(rule); got.Engine != "objdump" || got.Sandbox != "host" {
		t.Errorf("what was asked for stays: %+v", got)
	}
}

func TestTheEngineStageIsFollowedByTriage(t *testing.T) {
	r := engineRig(t, "MZ\x90\x00 image", "dendrite-rea/rizin:1")
	r.Functions = r.Inspector.(*CoreInspector).Client
	res, err := r.Analyze(ctx(), r.binary, Options{ID: "eng-tri"})
	if err != nil || res.State.EngineRan != "rizin" || res.State.Triage == nil || res.State.Stage != StageDone {
		t.Errorf("%+v, %v", res.State, err)
	}
}

func TestFunctionDocumentsAreNamedAfterTheirAddress(t *testing.T) {
	long := strings.Repeat("a", maxNameChars+20)
	for _, tc := range []struct {
		fn   engine.Function
		want string
	}{
		{engine.Function{Name: "main", Address: "0x1"}, "functions/0x1-main.md"},
		{engine.Function{Name: "std::vec<T>::push", Address: "0x2"}, "functions/0x2-std_vec_t_push.md"},
		{engine.Function{Name: "??", Address: "0x3"}, "functions/0x3-function.md"},
		{engine.Function{Name: long, Address: "0x4"}, "functions/0x4-" + long[:maxNameChars] + ".md"},
		{engine.Function{Name: "sym.imp.puts", Address: "0x5"}, "functions/0x5-sym.imp.puts.md"},
	} {
		if got := functionDocName(tc.fn); got != tc.want {
			t.Errorf("%+v: %s, want %s", tc.fn, got, tc.want)
		}
	}
	doc := functionDoc(engine.Function{Name: "f", Address: "0x1", Size: 9, Cyclomatic: 4, Calls: []string{"0x2"}, Disassembly: "nop\n", Pseudocode: "return;"})
	for _, want := range []string{"# Function f", "Cyclomatic complexity: 4", "Calls: 0x2", "```asm\nnop\n```", "```c\nreturn;\n```"} {
		if !strings.Contains(doc, want) {
			t.Errorf("document lacks %q:\n%s", want, doc)
		}
	}
	if bare := functionDoc(engine.Function{Name: "g", Address: "0x1"}); strings.Contains(bare, "Disassembly") || strings.Contains(bare, "Cyclomatic") {
		t.Errorf("a bare function has only its facts:\n%s", bare)
	}
}

func TestOnlyTheLargestFunctionsGetADocument(t *testing.T) {
	dir := t.TempDir()
	var fns []engine.Function
	for i := 0; i < maxFunctionDocs+5; i++ {
		fns = append(fns, engine.Function{Name: "f", Address: "0x" + strings.Repeat("1", 1) + string(rune('a'+i%26)) + string(rune('a'+i/26)), Size: uint64(1000 - i)})
	}
	if err := writeFunctionDocs(dir, fns); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "functions"))
	if len(entries) != maxFunctionDocs {
		t.Errorf("%d documents, want %d", len(entries), maxFunctionDocs)
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFunctionDocs(filepath.Join(blocked, "ws"), fns[:1]); err == nil {
		t.Error("a workspace that cannot be written is an error")
	}
}

func TestEngineFunctionsOfAnAnalysisWithoutAnEngineStageAreNone(t *testing.T) {
	r := newRig(t)
	finished(t, r, "eng-old")
	if fns, err := r.EngineFunctions("eng-old"); err != nil || fns != nil {
		t.Errorf("%+v, %v", fns, err)
	}
	if err := os.WriteFile(r.enginePath("eng-old"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.EngineFunctions("eng-old"); err == nil {
		t.Error("a corrupt engine.json is an error")
	}
}

// lockingEngine runs a real engine, then makes a directory read-only so the stage cannot save what it found.
type lockingEngine struct {
	EngineRunner
	lock func()
}

func (l lockingEngine) Run(c context.Context, target, format string, p engine.Policy) (*engine.Result, error) {
	res, err := l.EngineRunner.Run(c, target, format, p)
	l.lock()
	return res, err
}

func TestAnEngineStageThatCannotSaveItsResultIsAnError(t *testing.T) {
	for name, dirOf := range map[string]func(r *rig, id string) string{
		"engine.json": func(r *rig, id string) string { return r.Store.Dir(id) },
		"documents":   func(r *rig, id string) string { return r.Store.WorkspaceDir(id) },
	} {
		r := engineRig(t, "\x7fELF", "dendrite-rea/rizin:1")
		stops := false
		r.Engines = lockingEngine{EngineRunner: r.Engines, lock: func() { stops = readOnly(t, dirOf(r, "eng-ro")) }}
		if _, err := r.Analyze(ctx(), r.binary, Options{ID: "eng-ro"}); (err != nil) != stops {
			t.Errorf("%s: permissions stop writing = %t but err = %v", name, stops, err)
		}
	}
}
