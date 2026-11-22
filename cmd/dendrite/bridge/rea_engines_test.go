package bridge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/directive"
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/rea/engine"
	"Dendrite/pkg/rea/engine/enginetest"
	"Dendrite/pkg/testkit"
	"path/filepath"
	"strings"
	"testing"
)

// withEngines points the commands at stand-in docker and engines, with the given images "built", and lets the
// analysis choose the engine itself (the helper of the other tests turns engines off).
func withEngines(t *testing.T, images ...string) string {
	t.Helper()
	pe := useReaSidecars(t)
	t.Setenv(engine.EnvEngine, "")
	enginetest.UsePath(t, images...)
	return pe
}

func TestRetroAnalyzeRunsTheBestUsableEngineInTheContainer(t *testing.T) {
	pe := withEngines(t, "dendrite-rea/rizin:1")
	out, code := cliOutput(t, "retro", "analyze", pe, "--id", "e-auto")
	if code != cli.ExitOK || !strings.Contains(out, "Engine: rizin (container)") {
		t.Fatalf("auto: exit %d\n%s", code, out)
	}
	if out, code = cliOutput(t, "retro", "search", "e-auto", "push rbp"); code != cli.ExitOK || !strings.Contains(out, "functions/0x401000-main.md") {
		t.Errorf("the function documents are searchable: exit %d\n%s", code, out)
	}
}

func TestRetroAnalyzeSaysWhyNoEngineRanOrWasChosen(t *testing.T) {
	pe := withEngines(t) // no image is built
	out, code := cliOutput(t, "retro", "analyze", pe, "--id", "e-none")
	if code != cli.ExitOK || !strings.Contains(out, "Engine: none: no engine is usable in container mode") || !strings.Contains(out, "dendrite retro engines build rizin") {
		t.Errorf("nothing usable: exit %d\n%s", code, out)
	}
	if out, code = cliOutput(t, "retro", "analyze", pe, "--id", "e-off", "--engine", "none"); code != cli.ExitOK || !strings.Contains(out, "disabled") {
		t.Errorf("--engine none: exit %d\n%s", code, out)
	}
	t.Setenv(engine.EnvEngine, "none")
	if out, code = cliOutput(t, "retro", "analyze", pe, "--id", "e-env"); code != cli.ExitOK || !strings.Contains(out, "disabled") {
		t.Errorf("the environment sets the default: exit %d\n%s", code, out)
	}
	if _, code = cliOutput(t, "retro", "analyze", pe, "--id", "e-bad", "--engine", "ida"); code != cli.ExitError {
		t.Errorf("an unknown engine: exit %d", code)
	}
}

func TestRetroAnalyzeNeverOpensAnUntrustedBinaryOnTheHost(t *testing.T) {
	pe := withEngines(t)
	if out, code := cliOutput(t, "retro", "analyze", pe, "--id", "e-host", "--sandbox", "host"); code != cli.ExitError || !strings.Contains(out, "--trusted") {
		t.Errorf("an untrusted binary on the host: exit %d\n%s", code, out)
	}
	out, code := cliOutput(t, "retro", "resume", "e-host", "--trusted")
	if code != cli.ExitOK || !strings.Contains(out, "Engine: ghidra (host)") {
		t.Errorf("--trusted: exit %d\n%s", code, out)
	}
}

func TestRetroResumeSwitchesTheEngine(t *testing.T) {
	pe := withEngines(t, "dendrite-rea/rizin:1", "dendrite-rea/objdump:1")
	if out, code := cliOutput(t, "retro", "analyze", pe, "--id", "e-sw"); code != cli.ExitOK || !strings.Contains(out, "rizin (container)") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "retro", "resume", "e-sw", "--engine", "objdump"); code != cli.ExitOK || !strings.Contains(out, "Engine: objdump (container)") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestRetroEnginesListsWhereEachCanRun(t *testing.T) {
	withEngines(t, "dendrite-rea/rizin:1")
	out, code := cliOutput(t, "retro", "engines")
	if code != cli.ExitOK {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{"ENGINE", "rizin", "LGPL-3.0", "functions,disassembly,graph", "container: image dendrite-rea/radare2:1 is not built (dendrite retro engines build radare2)", "ghidra", "hopper", "proprietary tool, documented only (docs/features/rea-engines.md)"} {
		if !strings.Contains(out, want) {
			t.Errorf("the list lacks %q:\n%s", want, out)
		}
	}
	if out, code = cliOutput(t, "retro", "engines", "--json"); code != cli.ExitOK || !strings.Contains(out, `"usable": true`) || !strings.Contains(out, `"license": "LGPL-3.0"`) {
		t.Errorf("--json: exit %d\n%s", code, out)
	}
}

func TestRetroEnginesBuildIsExplicitAndReportsFailures(t *testing.T) {
	withEngines(t)
	if out, code := cliOutput(t, "retro", "engines", "build", "rizin"); code != cli.ExitOK || !strings.Contains(out, "Built the image of rizin") {
		t.Errorf("build: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "retro", "engines", "build", "ghidra"); code != cli.ExitError || !strings.Contains(out, "--sha256") {
		t.Errorf("ghidra needs a pinned release: exit %d\n%s", code, out)
	}
	pinned := []string{"retro", "engines", "build", "ghidra", "--url", "https://example.org/ghidra.zip", "--sha256", strings.Repeat("cd", 32)}
	if out, code := cliOutput(t, pinned...); code != cli.ExitOK || !strings.Contains(out, "Built the image of ghidra") {
		t.Errorf("ghidra with a pinned release: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "retro", "engines", "build", "ida"); code != cli.ExitError || !strings.Contains(out, "no engine image to build") {
		t.Errorf("a proprietary tool has no image: exit %d\n%s", code, out)
	}
	t.Setenv("STANDIN_MODE", "buildfail")
	if out, code := cliOutput(t, "retro", "engines", "build", "rizin"); code != cli.ExitError || !strings.Contains(out, "package not found") {
		t.Errorf("a failing build shows docker's message: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "retro", "engines", "build"); code != cli.ExitUsage {
		t.Errorf("build needs an engine name: exit %d", code)
	}
}

func TestEngineDefaultsFallBackWhenTheDirectiveCannotBeRead(t *testing.T) {
	t.Setenv(engine.EnvEngine, "")
	t.Setenv(engine.EnvSandbox, "")
	dir := t.TempDir() // not a repository: its own directives apply, and these cannot be read
	testkit.WriteFile(t, filepath.Join(dir, filepath.FromSlash(directive.Path)), "not = [valid", 0o644)
	t.Chdir(dir)
	if o := engineDefaults(analysis.Options{}); o.Engine != "auto" || o.Sandbox != "container" {
		t.Errorf("%+v", o)
	}
}

func TestEngineLineSaysWhichEngineRanOrWhyNone(t *testing.T) {
	for st, want := range map[string]analysis.State{
		"rizin (host)":  {EngineRan: "rizin", EngineMode: "host"},
		"none: no luck": {EngineNote: "no luck"},
		"not run":       {},
	} {
		if got := engineLine(want); got != st {
			t.Errorf("%+v: %q, want %q", want, got, st)
		}
	}
}
