package bridge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/coreclient/coretest"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/rea/analysis"
	"context"
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
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRetroAnalyzeThenSearchThenPurge(t *testing.T) {
	pe := useReaSidecars(t)
	out, code := cliOutput(t, "retro", "analyze", pe, "--id", "pe-cli", "--tier", "local")
	if code != cli.ExitOK || !strings.Contains(out, "Analysis pe-cli: done") || !strings.Contains(out, "KERNEL32.dll") || !strings.Contains(out, "retro search pe-cli") {
		t.Fatalf("analyze: exit %d\n%s", code, out)
	}
	out, code = cliOutput(t, "retro", "search", "pe-cli", "CreateFileW", "-k", "3")
	if code != cli.ExitOK || !strings.Contains(out, "--- imports/kernel32.dll.md") {
		t.Fatalf("search: exit %d\n%s", code, out)
	}
	if out, code = cliOutput(t, "rea", "search", "pe-cli", "CreateFileW", "--json"); code != cli.ExitOK || !strings.Contains(out, `"file": "imports/kernel32.dll.md"`) {
		t.Errorf("search --json: exit %d\n%s", code, out)
	}
	if _, code = cliOutput(t, "retro", "purge", "pe-cli"); code != cli.ExitError {
		t.Errorf("purge without --force: exit %d", code)
	}
	if out, code = cliOutput(t, "retro", "purge", "pe-cli", "--force"); code != cli.ExitOK || !strings.Contains(out, "deleted") {
		t.Errorf("purge --force: exit %d\n%s", code, out)
	}
	if _, code = cliOutput(t, "retro", "search", "pe-cli", "x"); code != cli.ExitError {
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
	bad := writePE(t, "ELF")
	if _, code := cliOutput(t, "retro", "analyze", bad, "--id", "pe-r"); code != cli.ExitError {
		t.Fatalf("a file that is not a PE image: exit %d", code)
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

// pauseDuringParse pauses its analysis while the image is parsed, as another process could.
type pauseDuringParse struct {
	inner analysis.Parser
	svc   *analysis.Service
}

func (p pauseDuringParse) PEParse(c context.Context, path string) (*coreclient.PEInfo, error) {
	info, err := p.inner.PEParse(c, path)
	if err == nil {
		_, err = p.svc.Pause("pe-pz")
	}
	return info, err
}

func TestRetroReportsAPauseAsAnOutcome(t *testing.T) {
	pe := useReaSidecars(t)
	real, err := analysis.NewService()
	if err != nil {
		t.Fatal(err)
	}
	real.Parser = pauseDuringParse{inner: real.Parser, svc: real}
	old := newReaService
	newReaService = func() (*analysis.Service, error) { return real, nil }
	t.Cleanup(func() { newReaService = old })

	out, code := cliOutput(t, "retro", "analyze", pe, "--id", "pe-pz")
	if code != cli.ExitOK || !strings.Contains(out, "is paused after stage parsed") || !strings.Contains(out, "retro resume pe-pz") {
		t.Errorf("a pause is not a failure: exit %d\n%s", code, out)
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
