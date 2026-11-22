package bridge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/coreclient/coretest"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/testkit"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useReaSidecars points the analysis at the stand-in sidecars and a temporary Dendrite home, and returns a PE image.
func useReaSidecars(t *testing.T) string {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	coretest.UseStandin(t)
	ragtest.UseStandin(t, "1.11.0")
	t.Cleanup(rag.CloseSidecars)
	return writePE(t, "MZ\x90\x00 pe image")
}

func writePE(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.exe")
	testkit.WriteFile(t, path, content, 0o600)
	return path
}

func TestRetroAnalyzeThenSearch(t *testing.T) {
	pe := useReaSidecars(t)
	out, code := cliOutput(t, "retro", "analyze", pe, "--id", "pe-cli", "--tier", "local")
	if code != cli.ExitOK || !strings.Contains(out, "Analysis pe-cli: done") || !strings.Contains(out, "KERNEL32.dll") || !strings.Contains(out, "retro search pe-cli") {
		t.Fatalf("analyze: exit %d\n%s", code, out)
	}
	out, code = cliOutput(t, "retro", "search", "pe-cli", "CreateFileW", "-k", "3")
	if code != cli.ExitOK || !strings.Contains(out, "--- libraries/kernel32.dll.md") {
		t.Fatalf("search: exit %d\n%s", code, out)
	}
	if out, code = cliOutput(t, "rea", "search", "pe-cli", "CreateFileW", "--json"); code != cli.ExitOK || !strings.Contains(out, `"file": "libraries/kernel32.dll.md"`) {
		t.Errorf("search --json: exit %d\n%s", code, out)
	}
}

func TestRetroPurgeNeedsForceAndDeletesTheAnalysis(t *testing.T) {
	pe := useReaSidecars(t)
	if _, code := cliOutput(t, "retro", "analyze", pe, "--id", "pe-del"); code != cli.ExitOK {
		t.Fatalf("analyze: exit %d", code)
	}
	if _, code := cliOutput(t, "retro", "purge", "pe-del"); code != cli.ExitError {
		t.Errorf("purge without --force: exit %d", code)
	}
	if out, code := cliOutput(t, "retro", "purge", "pe-del", "--force"); code != cli.ExitOK || !strings.Contains(out, "deleted") {
		t.Errorf("purge --force: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "retro", "search", "pe-del", "x"); code != cli.ExitError {
		t.Errorf("search a purged analysis: exit %d", code)
	}
}

func TestRetroAnalyzeJSONAndMatchlessSearch(t *testing.T) {
	pe := useReaSidecars(t)
	if out, code := cliOutput(t, "retro", "analyze", pe, "--id", "pe-j", "--json"); code != cli.ExitOK || !strings.Contains(out, `"stage": "done"`) || !strings.Contains(out, `"token_estimate"`) {
		t.Fatalf("analyze --json: exit %d\n%s", code, out)
	}
	t.Setenv("STANDIN_FAIL", "")
	if out, code := cliOutput(t, "retro", "search", "pe-j", "qqqqqqqq-not-in-any-document"); code != cli.ExitOK || !strings.Contains(out, "No passage matches") {
		t.Errorf("a query nothing matches: exit %d\n%s", code, out)
	}
}

func TestRetroResumeAfterAFailedParseAndPause(t *testing.T) {
	useReaSidecars(t)
	bad := writePE(t, "plain text")
	if _, code := cliOutput(t, "retro", "analyze", bad, "--id", "pe-r", "--format", "pe"); code != cli.ExitError {
		t.Fatalf("a target that is not the expected format: exit %d", code)
	}
	if out, code := cliOutput(t, "retro", "pause", "pe-r"); code != cli.ExitOK || !strings.Contains(out, "will pause after stage new") {
		t.Errorf("pause: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "retro", "pause", "pe-r", "--json"); code != cli.ExitOK || !strings.Contains(out, `"paused": true`) {
		t.Errorf("pause --json: exit %d\n%s", code, out)
	}
	if err := os.WriteFile(bad, []byte("MZ fixed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, code := cliOutput(t, "retro", "resume", "pe-r", "--tier", "paid", "--model", "cloud-pro"); code != cli.ExitOK || !strings.Contains(out, "Analysis pe-r: done") || !strings.Contains(out, "tier paid") {
		t.Errorf("resume: exit %d\n%s", code, out)
	}
}

func TestReportRunTreatsAPauseAsAnOutcome(t *testing.T) {
	res := &analysis.Result{State: analysis.State{ID: "pe-pz", Stage: analysis.StageParsed, Paused: true}}
	var out strings.Builder
	if err := reportRun(&out, res, analysis.ErrPaused); err != nil || !strings.Contains(out.String(), "is paused after stage parsed") || !strings.Contains(out.String(), "retro resume pe-pz") {
		t.Errorf("a pause is not a failure: %v\n%s", err, out.String())
	}
}

func TestRetroAnalysisCommandsReportUsageAndServiceErrors(t *testing.T) {
	useReaSidecars(t)
	for _, args := range [][]string{
		{"retro", "analyze"}, {"retro", "pause"}, {"retro", "resume"}, {"retro", "purge"}, {"retro", "search", "only-id"},
	} {
		if _, code := cliOutput(t, args...); code != cli.ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, cli.ExitUsage)
		}
	}
	for _, args := range [][]string{
		{"retro", "analyze", filepath.Join(t.TempDir(), "missing.exe")},
		{"retro", "pause", "pe-nope"}, {"retro", "resume", "pe-nope"}, {"retro", "purge", "pe-nope", "--force"},
	} {
		if _, code := cliOutput(t, args...); code != cli.ExitError {
			t.Errorf("%v: exit %d, want %d", args, code, cli.ExitError)
		}
	}

	old := newReaService
	newReaService = func() (*analysis.Service, error) { return nil, errors.New("no Dendrite home") }
	t.Cleanup(func() { newReaService = old })
	if _, code := cliOutput(t, "retro", "pause", "pe-x"); code != cli.ExitError {
		t.Errorf("a service that cannot be built: exit %d", code)
	}
}

func TestRetroExportWritesRustUnderOutput(t *testing.T) {
	pe := useReaSidecars(t)
	if _, code := cliOutput(t, "retro", "analyze", pe, "--id", "pe-exp"); code != cli.ExitOK {
		t.Fatalf("analyze: exit %d", code)
	}
	out := t.TempDir()
	text, code := cliOutput(t, "retro", "export", "pe-exp", "--target-language", "rust", "--output", out)
	if code != cli.ExitOK || !strings.Contains(text, filepath.Join(out, "pe-exp.rs")) {
		t.Fatalf("export: exit %d\n%s", code, text)
	}
	if _, err := os.Stat(filepath.Join(out, "pe-exp.rs")); err != nil {
		t.Error(err)
	}
	if _, code = cliOutput(t, "retro", "export", "pe-exp", "--output", out, "--json"); code != cli.ExitOK {
		t.Errorf("export --json: exit %d", code)
	}
	for _, args := range [][]string{
		{"retro", "export", "pe-exp", "--target-language", "go", "--output", out},
		{"retro", "export", "pe-exp"},
		{"retro", "export", "pe-none", "--output", out},
	} {
		if _, code = cliOutput(t, args...); code != cli.ExitError {
			t.Errorf("%v: exit %d, want an error", args, code)
		}
	}
}

func TestRetroAnalyzeIsNotLimitedToWindows(t *testing.T) {
	useReaSidecars(t)
	elf := writePE(t, "\x7fELF\x02\x01")
	out, code := cliOutput(t, "retro", "analyze", elf, "--id", "elf-cli")
	if code != cli.ExitOK || !strings.Contains(out, "(elf, tier local)") || !strings.Contains(out, "libc.so.6") {
		t.Fatalf("an ELF target: exit %d\n%s", code, out)
	}
	out, code = cliOutput(t, "retro", "analyze", t.TempDir(), "--id", "dir-cli", "--format", "source-tree")
	if code != cli.ExitOK || !strings.Contains(out, "(source-tree, tier local)") {
		t.Errorf("a directory: exit %d\n%s", code, out)
	}
	if out, code = cliOutput(t, "retro", "search", "elf-cli", "strcmp"); code != cli.ExitOK || !strings.Contains(out, "--- libraries/libc.so.6.md") {
		t.Errorf("search an ELF analysis: exit %d\n%s", code, out)
	}
}
