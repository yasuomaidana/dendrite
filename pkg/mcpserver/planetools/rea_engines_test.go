package planetools_test

import (
	"Dendrite/pkg/directive"
	"Dendrite/pkg/rea/engine"
	"Dendrite/pkg/rea/engine/enginetest"
	"Dendrite/pkg/testkit"
	"path/filepath"
	"strings"
	"testing"
)

func TestReaEnginesToolListsWhereEachEngineCanRun(t *testing.T) {
	s, _ := reaTools(t, nil)
	enginetest.UsePath(t, "dendrite-rea/rizin:1")
	out, err := callRea(t, s, "dendrite_rea_engines", map[string]any{})
	if err != nil || !strings.Contains(out, `"name": "rizin"`) || !strings.Contains(out, `"license": "LGPL-3.0"`) || !strings.Contains(out, `"usable": true`) ||
		!strings.Contains(out, "dendrite retro engines build radare2") {
		t.Errorf("%v\n%s", err, out)
	}
}

func TestReaAnalyzeRunsTheEngineChosenByItsArguments(t *testing.T) {
	s, pe := reaTools(t, nil)
	t.Setenv(engine.EnvEngine, "")
	enginetest.UsePath(t, "dendrite-rea/rizin:1", "dendrite-rea/objdump:1")
	out, err := callRea(t, s, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-eng"})
	if err != nil || !strings.Contains(out, `"engine_ran": "rizin"`) || !strings.Contains(out, `"engine_mode": "container"`) {
		t.Fatalf("auto: %v\n%s", err, out)
	}
	if out, err = callRea(t, s, "dendrite_rea_resume", map[string]any{"id": "pe-eng", "engine": "objdump"}); err != nil || !strings.Contains(out, `"engine_ran": "objdump"`) {
		t.Errorf("resume with another engine: %v\n%s", err, out)
	}
}

func TestReaAnalyzeRefusesAnUntrustedBinaryOnTheHostUnlessTrusted(t *testing.T) {
	s, pe := reaTools(t, nil)
	t.Setenv(engine.EnvEngine, "")
	enginetest.UsePath(t)
	expectToolError(t, s, "untrusted on the host", "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-host", "sandbox": "host"}, "--trusted")
	out, err := callRea(t, s, "dendrite_rea_resume", map[string]any{"id": "pe-host", "trusted": true})
	if err != nil || !strings.Contains(out, `"engine_mode": "host"`) || !strings.Contains(out, `"trusted": true`) {
		t.Errorf("--trusted: %v\n%s", err, out)
	}
}

func TestReaEngineDefaultsSurviveAnUnreadableDirective(t *testing.T) {
	s, pe := reaTools(t, nil)
	t.Setenv(engine.EnvEngine, "")
	enginetest.UsePath(t)
	dir := t.TempDir() // not a repository, with directives that cannot be read
	testkit.WriteFile(t, filepath.Join(dir, filepath.FromSlash(directive.Path)), "not = [valid", 0o644)
	t.Chdir(dir)
	out, err := callRea(t, s, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-def"})
	if err != nil || !strings.Contains(out, "no engine is usable in container mode") {
		t.Errorf("%v\n%s", err, out)
	}
}
