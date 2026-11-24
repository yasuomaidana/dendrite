package planetools_test

import (
	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/coreclient/coretest"
	"Dendrite/pkg/directive"
	. "Dendrite/pkg/mcpserver"
	"Dendrite/pkg/mcpserver/planetools"
	"Dendrite/pkg/planesvc/planetest"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/rea/analysis"
	"Dendrite/pkg/rea/engine"
	"Dendrite/pkg/rea/engine/enginetest"
	"Dendrite/pkg/testkit"
	"Dendrite/pkg/tracker/plane"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// reaTools is a server with the analysis tools over the stand-in sidecars and a temporary Dendrite home.
func reaTools(t *testing.T, service func() (*analysis.Service, error)) (*Server, string) {
	t.Helper()
	return reaToolsWith(t, Dependencies{}, service)
}

// reaToolsWith is reaTools with the dependencies the tools use for tickets.
func reaToolsWith(t *testing.T, deps Dependencies, service func() (*analysis.Service, error)) (*Server, string) {
	t.Helper()
	t.Setenv("DENDRITE_HOME", t.TempDir())
	t.Setenv(engine.EnvEngine, engine.EngineNone) // these tests are about the analysis; engines have their own
	coretest.UseStandin(t)
	ragtest.UseStandin(t, "1.11.0")
	t.Cleanup(rag.CloseSidecars)
	pe := filepath.Join(t.TempDir(), "sample.exe")
	if err := os.WriteFile(pe, []byte("MZ\x90\x00 pe image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if service == nil {
		service = analysis.NewService
	}
	s := NewServer(ServerConfig{})
	planetools.RegisterReaAnalysisTools(s, deps, service)
	planetools.RegisterReaInsightTools(s, service)
	return s, pe
}

// expectToolError calls a tool and requires an error that holds want.
func expectToolError(t *testing.T, s *Server, name, tool string, args map[string]any, want string) {
	t.Helper()
	if _, err := callRea(t, s, tool, args); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("%s: %v, want %q", name, err, want)
	}
}

func callRea(t *testing.T, s *Server, tool string, args map[string]any) (string, error) {
	t.Helper()
	h, ok := s.Handler(tool)
	if !ok {
		t.Fatalf("%s is not registered", tool)
	}
	res, err := h(context.Background(), args)
	if err != nil {
		return "", err
	}
	return res.Content[0].Text, nil
}

func TestReaToolsRunAnAnalysisSearchAndResume(t *testing.T) {
	s, pe := reaTools(t, nil)
	out, err := callRea(t, s, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-mcp"})
	if err != nil || !strings.Contains(out, `"stage": "done"`) || !strings.Contains(out, "KERNEL32.dll") {
		t.Fatalf("analyze: %v\n%s", err, out)
	}
	if out, err = callRea(t, s, "dendrite_rea_search", map[string]any{"id": "pe-mcp", "query": "CreateFileW", "k": float64(2)}); err != nil || !strings.Contains(out, "libraries/kernel32.dll.md") {
		t.Errorf("search: %v\n%s", err, out)
	}
	if out, err = callRea(t, s, "dendrite_rea_resume", map[string]any{"id": "pe-mcp", "tier": "paid"}); err != nil || !strings.Contains(out, `"tier": "paid"`) {
		t.Errorf("resume: %v\n%s", err, out)
	}
}

func TestReaPurgeToolNeedsForce(t *testing.T) {
	s, pe := reaTools(t, nil)
	if _, err := callRea(t, s, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-purge"}); err != nil {
		t.Fatal(err)
	}
	if _, err := callRea(t, s, "dendrite_rea_purge", map[string]any{"id": "pe-purge", "force": false}); !errors.Is(err, analysis.ErrNeedsForce) {
		t.Errorf("purge without force: %v", err)
	}
	if out, err := callRea(t, s, "dendrite_rea_purge", map[string]any{"id": "pe-purge", "force": true}); err != nil || !strings.Contains(out, `"purged": "pe-purge"`) {
		t.Errorf("purge: %v\n%s", err, out)
	}
}

// pauseDuringParse pauses its analysis while the image is parsed, as another process could.
type pauseDuringParse struct {
	inner analysis.Inspector
	svc   *analysis.Service
}

func (p pauseDuringParse) Inspect(c context.Context, path string) (*coreclient.Image, error) {
	info, err := p.inner.Inspect(c, path)
	if err == nil {
		_, err = p.svc.Pause("pe-pz")
	}
	return info, err
}

func TestReaToolsReturnAPauseAsAResult(t *testing.T) {
	var svc *analysis.Service
	s, pe := reaTools(t, func() (*analysis.Service, error) { return svc, nil })
	svc, _ = analysis.NewService()
	svc.Inspector = pauseDuringParse{inner: svc.Inspector, svc: svc}

	out, err := callRea(t, s, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-pz"})
	if err != nil || !strings.Contains(out, `"paused": true`) || !strings.Contains(out, `"stage": "parsed"`) {
		t.Errorf("a pause is a result: %v\n%s", err, out)
	}
	if out, err = callRea(t, s, "dendrite_rea_pause", map[string]any{"id": "pe-pz"}); err != nil || !strings.Contains(out, `"paused": true`) {
		t.Errorf("pause: %v\n%s", err, out)
	}
	svc.Inspector = svc.Inspector.(pauseDuringParse).inner
	if out, err = callRea(t, s, "dendrite_rea_resume", map[string]any{"id": "pe-pz"}); err != nil || !strings.Contains(out, `"stage": "done"`) {
		t.Errorf("resume after the pause: %v\n%s", err, out)
	}
}

func TestReaToolsReportWhatTheyCannotDo(t *testing.T) {
	s, pe := reaTools(t, nil)
	for name, tc := range map[string]struct {
		tool string
		args map[string]any
		want string
	}{
		"no binary":      {"dendrite_rea_analyze", map[string]any{}, "binary is required"},
		"no id":          {"dendrite_rea_pause", map[string]any{}, "id is required"},
		"no force":       {"dendrite_rea_purge", map[string]any{"id": "x"}, "force is required"},
		"no query":       {"dendrite_rea_search", map[string]any{"id": "x"}, "query is required"},
		"unknown tier":   {"dendrite_rea_analyze", map[string]any{"binary": pe, "tier": "free"}, "unknown tier"},
		"unknown id":     {"dendrite_rea_resume", map[string]any{"id": "pe-none"}, "no such analysis"},
		"unknown purge":  {"dendrite_rea_purge", map[string]any{"id": "pe-none", "force": true}, "no such analysis"},
		"search unknown": {"dendrite_rea_search", map[string]any{"id": "pe-none", "query": "x"}, "no such analysis"},
		"pause unknown":  {"dendrite_rea_pause", map[string]any{"id": "pe-none"}, "no such analysis"},
	} {
		expectToolError(t, s, name, tc.tool, tc.args, tc.want)
	}

	broken, _ := reaTools(t, func() (*analysis.Service, error) { return nil, errors.New("no Dendrite home") })
	if _, err := callRea(t, broken, "dendrite_rea_pause", map[string]any{"id": "x"}); err == nil || !strings.Contains(err.Error(), "no Dendrite home") {
		t.Errorf("a service that cannot be built: %v", err)
	}
}

// analyzedTools is a server with the REA tools and a finished analysis named pe-ins.
func analyzedTools(t *testing.T) *Server {
	t.Helper()
	s, pe := reaTools(t, nil)
	if _, err := callRea(t, s, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-ins"}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestReaReuseToolFindsWhatTheProjectHas(t *testing.T) {
	s := analyzedTools(t)
	project := t.TempDir()
	testkit.WriteFile(t, filepath.Join(project, "checksum.go"), "package main\n\nfunc ComputeChecksum() {}\n", 0o644)
	out, err := callRea(t, s, "dendrite_rea_reuse", map[string]any{"id": "pe-ins", "project": project, "k": float64(3)})
	if err != nil || !strings.Contains(out, "checksum.go") || !strings.Contains(out, `"source": "project"`) {
		t.Errorf("reuse: %v\n%s", err, out)
	}
}

func TestReaRankToolOrdersTheDocuments(t *testing.T) {
	out, err := callRea(t, analyzedTools(t), "dendrite_rea_rank", map[string]any{"id": "pe-ins"})
	if err != nil || !strings.Contains(out, `"doc": "overview.md"`) || !strings.Contains(out, "libraries/kernel32.dll.md") {
		t.Errorf("rank: %v\n%s", err, out)
	}
}

func TestReaADRToolDraftsAProposedRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "decisions")
	out, err := callRea(t, analyzedTools(t), "dendrite_rea_adr", map[string]any{"id": "pe-ins", "output": dir, "title": "Recreate it"})
	if err != nil || !strings.Contains(out, "ADR-001.md") || !strings.Contains(out, `"status": "proposed"`) {
		t.Errorf("adr: %v\n%s", err, out)
	}
}

func TestReaExportToolWritesAModuleInEachLanguage(t *testing.T) {
	tools := analyzedTools(t)
	for language, file := range map[string]string{"": "pe-ins.rs", "go": "pe-ins.go", "python": "pe-ins.py", "typescript": "pe-ins.ts"} {
		out, err := callRea(t, tools, "dendrite_rea_export", map[string]any{"id": "pe-ins", "language": language, "output": t.TempDir()})
		if err != nil || !strings.Contains(out, file) {
			t.Errorf("export %q: %v\n%s", language, err, out)
		}
	}
}

func TestReaInsightToolsReportWhatTheyCannotDo(t *testing.T) {
	s, _ := reaTools(t, nil)
	for name, tc := range map[string]struct {
		tool string
		args map[string]any
		want string
	}{
		"reuse needs an id":             {"dendrite_rea_reuse", map[string]any{}, "id is required"},
		"rank needs an id":              {"dendrite_rea_rank", map[string]any{}, "id is required"},
		"adr needs an id":               {"dendrite_rea_adr", map[string]any{}, "id is required"},
		"reuse of an unknown id":        {"dendrite_rea_reuse", map[string]any{"id": "pe-none"}, "no such analysis"},
		"rank of an unknown id":         {"dendrite_rea_rank", map[string]any{"id": "pe-none"}, "no such analysis"},
		"adr of an unknown id":          {"dendrite_rea_adr", map[string]any{"id": "pe-none"}, "no such analysis"},
		"export needs an output":        {"dendrite_rea_export", map[string]any{"id": "pe-none", "output": ""}, "output is required"},
		"export of an unknown id":       {"dendrite_rea_export", map[string]any{"id": "pe-none", "output": "/tmp/x"}, "no such analysis"},
		"export in an unknown language": {"dendrite_rea_export", map[string]any{"id": "pe-none", "language": "cobol", "output": "/tmp/x"}, "unsupported target language"},
	} {
		expectToolError(t, s, name, tc.tool, tc.args, tc.want)
	}
}

// ticketTools is a server whose tools reach a Plane test board holding DEN-1.
func ticketTools(t *testing.T) (*Server, *planetest.Board, string) {
	t.Helper()
	board := planetest.NewBoard(t, map[string]string{"DEN-1": "In Progress"})
	client, err := plane.NewClient(plane.Config{BaseURL: board.URL, WorkspaceSlug: planetest.BoardWorkspace, ProjectID: planetest.BoardProject, DirectToken: "k"}, board.Client())
	if err != nil {
		t.Fatal(err)
	}
	s, pe := reaToolsWith(t, Dependencies{PlaneClient: client}, nil)
	return s, board, pe
}

func TestReaAnalyzeLeavesANoteOnTheTicket(t *testing.T) {
	s, board, pe := ticketTools(t)
	out, err := callRea(t, s, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-n1", "ticket": "DEN-1"})
	if err != nil || !strings.Contains(out, `"ticket_note": "posted"`) || !strings.Contains(out, `"stage": "done"`) {
		t.Fatalf("a note on the ticket: %v\n%s", err, out)
	}
	if texts := board.Texts("DEN-1"); len(texts) != 1 || !strings.Contains(texts[0], "REA analysis pe-n1") {
		t.Errorf("comments %q", texts)
	}
}

func TestReaAnalyzeReportsATicketNoteThatCannotBePosted(t *testing.T) {
	s, board, pe := ticketTools(t)
	board.Fail("comments/", 0)
	out, err := callRea(t, s, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-n2", "ticket": "DEN-1"})
	if err != nil || !strings.Contains(out, `"ticket_note": "not posted:`) {
		t.Errorf("a ticket that cannot be commented on is reported in the result: %v\n%s", err, out)
	}
}

func TestReaAnalyzeWithoutAPlaneClientSaysWhyTheNoteWasNotPosted(t *testing.T) {
	t.Setenv("PLANE_API_KEY", "")
	t.Setenv("PLANE_BASE_URL", "")
	bare, pe := reaTools(t, nil)
	out, err := callRea(t, bare, "dendrite_rea_analyze", map[string]any{"binary": pe, "id": "pe-n3", "ticket": "DEN-1"})
	if err != nil || !strings.Contains(out, "not posted: plane client error") {
		t.Errorf("without a Plane client: %v\n%s", err, out)
	}
}

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
