package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/rea/engine/enginetest"
	"Dendrite/pkg/sandbox/sandboxtest"
	"Dendrite/pkg/testkit"
)

func target(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample")
	testkit.WriteFile(t, path, "\x7fELF", 0o600)
	return path
}

func logOf(t *testing.T) (string, func() string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("STANDIN_LOG", path)
	return path, func() string { data, _ := os.ReadFile(path); return string(data) }
}

func TestRizinInTheHardenedContainerListsFunctionsAndReadsTheLargest(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/rizin:1")
	res, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Engine != "rizin" || res.Mode != ModeContainer || len(res.Functions) != 3 {
		t.Fatalf("%+v", res)
	}
	checkMain(t, res.Functions[0])
	if res.Functions[1].Name != "helper" || res.Functions[2].Name != "tiny" {
		t.Errorf("functions are ordered by size: %+v", res.Functions)
	}
}

// checkMain checks the function the rizin stand-in lists first, with its calls and disassembly.
func checkMain(t *testing.T, main Function) {
	t.Helper()
	if main.Name != "main" || main.Address != "0x401000" || main.Size != 64 || main.Cyclomatic != 3 {
		t.Errorf("main: %+v", main)
	}
	if len(main.Calls) != 1 || main.Calls[0] != "0x401040" || !strings.Contains(main.Disassembly, "0x401000  push rbp") {
		t.Errorf("main calls and disassembly: %+v", main)
	}
}

func TestTheContainerRunsWithEveryHardeningFlagAndTheTargetReadOnly(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/rizin:1")
	_, calls := logOf(t)
	if _, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{}); err != nil {
		t.Fatal(err)
	}
	log := calls()
	testkit.Contains(t, log, "docker run", "--network none", "--cap-drop ALL", "--read-only", "no-new-privileges", ":/target/input:ro", "dendrite-rea/rizin:1 sh -c", "aaa;aflj")
	if strings.Index(log, "rizin -q") < strings.Index(log, "docker run") {
		t.Errorf("rizin runs only inside the container, after docker run:\n%s", log)
	}
}

func TestObjdumpInTheContainerReadsFunctionsOffItsLabels(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/objdump:1")
	res, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{Engine: "objdump"})
	if err != nil || res.Engine != "objdump" || len(res.Functions) != 2 {
		t.Fatalf("%+v, %v", res, err)
	}
	if m := res.Functions[0]; m.Name != "main" || m.Size != 7 || len(m.Calls) != 1 || m.Calls[0] != "helper" || !strings.Contains(m.Disassembly, "call   401040 <helper>") {
		t.Errorf("main: %+v", m)
	}
}

func TestATrustedBinaryMayRunOnTheHost(t *testing.T) {
	enginetest.UsePath(t)
	_, calls := logOf(t)
	res, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{Engine: "rizin", Sandbox: ModeHost, Trusted: true})
	if err != nil || res.Engine != "rizin" || res.Mode != ModeHost || len(res.Functions) != 3 {
		t.Fatalf("%+v, %v", res, err)
	}
	if log := calls(); strings.Contains(log, "docker run") || !strings.Contains(log, "rizin -q -N -c aaa;aflj") {
		t.Errorf("host mode runs the tool directly:\n%s", log)
	}
}

func TestTheHostRefusesAnUntrustedBinaryBeforeRunningAnything(t *testing.T) {
	enginetest.UsePath(t)
	_, calls := logOf(t)
	if _, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{Sandbox: ModeHost}); err == nil {
		t.Fatal("an untrusted binary on the host must be refused")
	}
	if calls() != "" {
		t.Errorf("nothing runs before the refusal:\n%s", calls())
	}
}

func TestWithNothingUsableTheResultSaysWhyAndIsNotAnError(t *testing.T) {
	enginetest.UsePath(t) // Docker answers but no image is built
	res, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{})
	if err != nil || res.Engine != "" || !strings.Contains(res.Note, "dendrite retro engines build rizin") {
		t.Errorf("%+v, %v", res, err)
	}
	t.Setenv("STANDIN_DOCKER", "down")
	if res, err = NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{}); err != nil || !strings.Contains(res.Note, "Docker does not answer") {
		t.Errorf("%+v, %v", res, err)
	}
	if res, err = NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{Engine: EngineNone}); err != nil || !strings.Contains(res.Note, "disabled") {
		t.Errorf("%+v, %v", res, err)
	}
}

func TestAnEngineThatFailsIsAnError(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/rizin:1", "dendrite-rea/objdump:1")
	t.Setenv("STANDIN_MODE", "r2broken")
	if _, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{}); err == nil || !strings.Contains(err.Error(), "did not answer a function list") || !strings.Contains(err.Error(), "rizin (container)") {
		t.Errorf("broken output: %v", err)
	}
	t.Setenv("STANDIN_MODE", "objfail")
	if _, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{Engine: "objdump"}); err == nil {
		t.Error("an engine that exits with an error is an error")
	}
	if _, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{Sandbox: "vm"}); err == nil {
		t.Error("a refused policy is an error")
	}
}

func TestOnlyTheLargestFunctionsAreReadAndTheListIsCapped(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/radare2:1")
	_, calls := logOf(t)
	t.Setenv("STANDIN_MODE", "many")
	res, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{})
	if err != nil || res.Engine != "radare2" || len(res.Functions) != maxFunctions {
		t.Fatalf("%d functions, %v", len(res.Functions), err)
	}
	if got := strings.Count(innerLine(calls(), "radare2 -q", "pdfj"), "pdfj @"); got != detailLimit {
		t.Errorf("the disassembly of %d functions was read, want %d", got, detailLimit)
	}
	if res.Functions[0].Size < res.Functions[len(res.Functions)-1].Size {
		t.Error("the largest functions come first")
	}
}

// innerLine is the line of the log where the tool itself ran (not the docker command that carried it) and holds word.
func innerLine(log, prefix, word string) string {
	for _, l := range strings.Split(log, "\n") {
		if strings.HasPrefix(l, prefix) && strings.Contains(l, word) {
			return l
		}
	}
	return ""
}

func TestAnEngineThatFindsNoFunctionsReadsNothingMore(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/rizin:1")
	_, calls := logOf(t)
	t.Setenv("STANDIN_MODE", "empty")
	res, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{})
	if err != nil || res.Engine != "rizin" || len(res.Functions) != 0 || strings.Contains(calls(), "pdfj") {
		t.Errorf("%+v, %v\n%s", res, err, calls())
	}
}

func TestCapOutputAndStdoutOfBoundWhatIsRead(t *testing.T) {
	if got := capOutput(make([]byte, maxOutput+5)); len(got) != maxOutput {
		t.Errorf("%d bytes kept", len(got))
	}
	if got := capOutput([]byte("short")); string(got) != "short" {
		t.Errorf("%q", got)
	}
	if stdoutOf(nil) != nil {
		t.Error("a run that never started printed nothing")
	}
}

func TestAnEngineThatCannotOpenTheTargetOrReadAFunctionIsAnError(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/rizin:1")
	if _, err := NewService().Run(context.Background(), filepath.Join(t.TempDir(), "missing"), coreclient.FormatELF, Policy{}); err == nil {
		t.Error("a target the engine cannot open is an error")
	}
	t.Setenv("STANDIN_MODE", "detailfail")
	if _, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{}); err == nil {
		t.Error("a failing disassembly run is an error")
	}
}

func TestShellLineQuotesEveryWord(t *testing.T) {
	if got := shellLine([]string{"rizin", "-c", "aaa;aflj", "it's"}); got != `'rizin' '-c' 'aaa;aflj' 'it'\''s'` {
		t.Errorf("%s", got)
	}
}

func TestParsersKeepWhatTheyCanRead(t *testing.T) {
	if got := parseR2Disassembly([]byte(`{"addr":1,"ops":[{"offset":1,"disasm":"nop"}]} {broken`)); got["0x1"] != "0x1  nop\n" {
		t.Errorf("a broken tail keeps what was read: %v", got)
	}
	if _, err := parseR2Functions([]byte(`{"not":"a list"}`)); err == nil {
		t.Error("an object is not a function list")
	}
	var sb strings.Builder
	sb.WriteString("0000000000001000 <big>:\n")
	for i := 0; i < maxDisasmLines+20; i++ {
		sb.WriteString("    10" + string(rune('0'+i%10)) + "0:\tnop\n")
	}
	fns := parseObjdump([]byte(sb.String()))
	if len(fns) != 1 || strings.Count(fns[0].Disassembly, "\n") != maxDisasmLines {
		t.Errorf("the disassembly of a function is capped at %d lines: %d functions", maxDisasmLines, len(fns))
	}
	if len(parseObjdump([]byte("no functions here\n"))) != 0 {
		t.Error("text without labels has no functions")
	}
}

func TestStatusesSayWhereEachEngineCanRun(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/rizin:1")
	by := map[string]Status{}
	for _, s := range Statuses(context.Background(), NewService().Env) {
		by[s.Spec.Name] = s
	}
	for name, want := range map[string]struct {
		container, host bool
		fix, reason     string
	}{
		"rizin":   {true, true, "", ""},
		"radare2": {false, true, "dendrite retro engines build radare2", ""},
		"ghidra":  {false, true, "dendrite retro engines build ghidra", ""},
		"ida":     {false, false, "", "proprietary tool, documented only"},
	} {
		got := by[name]
		if got.Container.Usable != want.container || got.Host.Usable != want.host || got.Container.Fix != want.fix || !strings.Contains(got.Container.Reason, want.reason) {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestBuildMakesTheImageFromTheEmbeddedDockerfile(t *testing.T) {
	enginetest.UsePath(t)
	_, calls := logOf(t)
	out, err := NewService().Build(context.Background(), "rizin", BuildOptions{})
	if err != nil || !strings.Contains(out, "built from debian:trixie-slim as dendrite-rea/rizin:1") || !strings.Contains(calls(), "docker build -t dendrite-rea/rizin:1 ") {
		t.Errorf("%q, %v\n%s", out, err, calls())
	}
	for _, name := range []string{"radare2", "objdump"} {
		if _, err := NewService().Build(context.Background(), name, BuildOptions{}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	t.Setenv("STANDIN_MODE", "buildfail")
	if out, err = NewService().Build(context.Background(), "rizin", BuildOptions{}); err == nil || !strings.Contains(out, "package not found") {
		t.Errorf("a failing build reports what docker printed: %q, %v", out, err)
	}
	for _, name := range []string{"ghidra", "ida"} {
		if _, err := NewService().Build(context.Background(), name, BuildOptions{}); err == nil {
			t.Errorf("%s has no image to build", name)
		}
	}
}

func TestBuildUsesTheRunnerItIsGivenAndReportsAnUnwritableTemporaryDirectory(t *testing.T) {
	runner := &sandboxtest.RecordingRunner{Output: []byte("ok")}
	out, err := (&Service{Exec: runner}).Build(context.Background(), "objdump", BuildOptions{})
	if err != nil || out != "ok" || !runner.HasCall(func(c []string) bool { return len(c) == 5 && c[1] == "build" && c[3] == "dendrite-rea/objdump:1" }) {
		t.Errorf("%q, %v", out, err)
	}
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	if _, err := (&Service{Exec: runner}).Build(context.Background(), "objdump", BuildOptions{}); err == nil {
		t.Error("no temporary directory is an error")
	}
}

func TestEveryImplementedEngineHasADockerfileForItsImage(t *testing.T) {
	for _, s := range Catalog() {
		data, err := dockerfiles.ReadFile("dockerfiles/" + s.Name + ".Dockerfile")
		if s.Implemented && (err != nil || !strings.Contains(string(data), "FROM ") || !strings.Contains(string(data), "USER 65534")) {
			t.Errorf("%s: %v\n%s", s.Name, err, data)
		}
	}
}

func TestAutoOnTheHostTakesTheBestEngineOnPath(t *testing.T) {
	enginetest.UsePath(t)
	res, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{Sandbox: ModeHost, Trusted: true})
	if err != nil || res.Engine != "ghidra" || res.Mode != ModeHost {
		t.Errorf("%+v, %v", res, err)
	}
}
