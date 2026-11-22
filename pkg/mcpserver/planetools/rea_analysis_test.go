package planetools_test

import (
	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/coreclient/coretest"
	"Dendrite/pkg/mcpserver/planetools"
	"Dendrite/pkg/rag"
	"Dendrite/pkg/rag/ragtest"
	"Dendrite/pkg/rea/analysis"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "Dendrite/pkg/mcpserver"
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
		if _, err := callRea(t, s, tc.tool, tc.args); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}

	broken, _ := reaTools(t, func() (*analysis.Service, error) { return nil, errors.New("no Dendrite home") })
	if _, err := callRea(t, broken, "dendrite_rea_pause", map[string]any{"id": "x"}); err == nil || !strings.Contains(err.Error(), "no Dendrite home") {
		t.Errorf("a service that cannot be built: %v", err)
	}
}
