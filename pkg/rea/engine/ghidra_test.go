package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/rea/engine/enginetest"
	"Dendrite/pkg/testkit"
)

func TestGhidraInTheContainerListsFunctionsWithPseudocode(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/ghidra:1")
	_, calls := logOf(t)
	res, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{Engine: "ghidra"})
	if err != nil || res.Engine != "ghidra" || res.Mode != ModeContainer || len(res.Functions) != 3 {
		t.Fatalf("%+v, %v", res, err)
	}
	main := res.Functions[0]
	if main.Name != "main" || main.Cyclomatic != 3 || len(main.Calls) != 1 {
		t.Errorf("main: %+v", main)
	}
	testkit.Contains(t, main.Pseudocode, "return helper();")
	testkit.Contains(t, main.Disassembly, "PUSH RBP")
	checkGhidraCommandLine(t, calls())
}

// checkGhidraCommandLine checks the container limits, environment and headless command of a Ghidra run.
func checkGhidraCommandLine(t *testing.T, log string) {
	t.Helper()
	testkit.Contains(t, log, "-m 4096m", "-e HOME=/tmp", "-e JAVA_TOOL_OPTIONS=-Duser.home=/tmp", "--network none", "--cap-drop ALL", ":/target/input:ro")
	testkit.Contains(t, log, "'analyzeHeadless' '/tmp' 'dendrite' '-import' '/target/input' '-scriptPath' '/scripts' '-postScript' 'ExportFunctions.java' '-deleteProject'")
}

func TestAutoTakesGhidraWhenItsImageIsBuiltBeforeRizin(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/rizin:1", "dendrite-rea/ghidra:1")
	res, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{})
	if err != nil || res.Engine != "ghidra" {
		t.Errorf("%+v, %v", res, err)
	}
}

func TestGhidraOnTheHostGetsTheScriptWrittenForIt(t *testing.T) {
	enginetest.UsePath(t)
	res, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{Engine: "ghidra", Sandbox: ModeHost, Trusted: true})
	if err != nil || res.Engine != "ghidra" || res.Mode != ModeHost || len(res.Functions) != 3 {
		t.Fatalf("%+v, %v", res, err)
	}
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	if _, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{Engine: "ghidra", Sandbox: ModeHost, Trusted: true}); err == nil {
		t.Error("no temporary directory for the script is an error")
	}
}

func TestGhidraResultsAreReadBetweenTheMarkersOnly(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/ghidra:1")
	for mode, want := range map[string]string{"ghidranomarkers": "between the markers", "ghidrabroken": "not a function list"} {
		t.Setenv("STANDIN_MODE", mode)
		_, err := NewService().Run(context.Background(), target(t), coreclient.FormatELF, Policy{Engine: "ghidra"})
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "ghidra (container)") {
			t.Errorf("%s: %v", mode, err)
		}
	}
	if _, err := parseGhidra([]byte(ghidraEnd + "\n" + ghidraBegin)); err == nil {
		t.Error("markers in the wrong order are no result")
	}
	fns, err := parseGhidra([]byte("INFO log\n" + ghidraBegin + "\n[]\n" + ghidraEnd + "\nINFO more log"))
	if err != nil || len(fns) != 0 {
		t.Errorf("an empty list between noise: %+v, %v", fns, err)
	}
}

func TestGhidraFailuresOfTheEngineItselfAreErrors(t *testing.T) {
	enginetest.UsePath(t, "dendrite-rea/ghidra:1")
	if _, err := NewService().Run(context.Background(), filepath.Join(t.TempDir(), "missing"), coreclient.FormatELF, Policy{Engine: "ghidra"}); err == nil {
		t.Error("a target Ghidra cannot import is an error")
	}
}

func TestGhidraBuildRefusesAReleaseThatIsNotPinned(t *testing.T) {
	enginetest.UsePath(t)
	_, calls := logOf(t)
	url := "https://example.org/ghidra_11_PUBLIC.zip"
	for name, o := range map[string]BuildOptions{
		"nothing":        {},
		"no checksum":    {URL: url},
		"plain http":     {URL: "http://example.org/g.zip", SHA256: strings.Repeat("ab", 32)},
		"a short digest": {URL: url, SHA256: "abc"},
	} {
		_, err := NewService().Build(context.Background(), "ghidra", o)
		if err == nil {
			t.Errorf("%s: must be refused", name)
			continue
		}
		testkit.Contains(t, err.Error(), "--url", "--sha256")
	}
	if calls() != "" {
		t.Errorf("nothing is built before the options are checked:\n%s", calls())
	}
}

func TestGhidraBuildPassesTheReleaseAsBuildArguments(t *testing.T) {
	enginetest.UsePath(t)
	_, calls := logOf(t)
	good := BuildOptions{URL: "https://example.org/ghidra_11_PUBLIC.zip", SHA256: strings.Repeat("ab", 32)}
	out, err := NewService().Build(context.Background(), "ghidra", good)
	if err != nil {
		t.Fatal(err)
	}
	testkit.Contains(t, out, "context holds ExportFunctions.java")
	testkit.Contains(t, calls(), "--build-arg GHIDRA_URL="+good.URL, "--build-arg GHIDRA_SHA256="+good.SHA256, "-t dendrite-rea/ghidra:1")
}

func TestTheGhidraImageVerifiesTheDownloadAndRunsAsNobody(t *testing.T) {
	dockerfile, err := dockerfiles.ReadFile("dockerfiles/ghidra.Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	script, err := dockerfiles.ReadFile("dockerfiles/ghidra/" + ghidraScript)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sha256sum -c", `test -n "$GHIDRA_URL"`, "USER 65534:65534", "COPY ExportFunctions.java /scripts/"} {
		if !strings.Contains(string(dockerfile), want) {
			t.Errorf("the Dockerfile lacks %q", want)
		}
	}
	for _, want := range []string{"extends GhidraScript", ghidraBegin, ghidraEnd, "DecompInterface", "BasicBlockModel", "edges - nodes + 2"} {
		if !strings.Contains(string(script), want) {
			t.Errorf("the export script lacks %q", want)
		}
	}
}
