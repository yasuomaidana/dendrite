package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"Dendrite/pkg/docusaurus"
)

func TestCodex_ReferenceGenerateAndAudit(t *testing.T) {
	docs := t.TempDir()
	if out, code := cliOutput(t, "codex", "audit", "--strict", "--docs", docs); code != ExitError || !strings.Contains(out, "REFERENCE DOCUMENTATION AUDIT FAILED") {
		t.Fatalf("strict audit of empty docs: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "codex", "audit", "--reference", "--docs", docs); code != ExitOK || !strings.Contains(out, "AUDIT FAILED") {
		t.Fatalf("non-strict audit reports but passes: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "docs", "generate", "--reference", "--docs", docs); code != ExitOK || !strings.Contains(out, "Reference documentation generated") {
		t.Fatalf("generate --reference: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "doc", "audit", "--strict", "--docs", docs); code != ExitOK || !strings.Contains(out, "audit passed") {
		t.Fatalf("strict audit after generate: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(docs, "cli", "codex.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCodex_SiteAuditAndReflect(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "intro.md"), []byte("---\nid: intro\ntitle: Intro\n---\n# Intro\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := cliOutput(t, "codex", "audit", "--root", root, "--docs", docs); code != ExitOK || !strings.Contains(out, "DOCUSAURUS DOCUMENTATION AUDIT SCORECARD") {
		t.Fatalf("site audit: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "codex", "audit", "--root", root, "--docs", docs, "--json"); code != ExitOK || !strings.Contains(out, "{") {
		t.Fatalf("site audit --json: exit %d\n%s", code, out)
	}
	target := filepath.Join(root, "reflected")
	if out, code := cliOutput(t, "codex", "reflect", "--reference", docs, "--source", root, "--target", target); code != ExitOK || !strings.Contains(out, "Reflected") {
		t.Fatalf("reflect: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "codex", "reflect", "--reference", filepath.Join(root, "missing"), "--target", target); code != ExitError {
		t.Fatalf("reflect from a missing dir: exit %d", code)
	}
}

func TestCodex_SiteLifecycle(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "intro.md"), []byte("---\nid: intro\ntitle: Intro\n---\n# Intro\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(root, "website")
	if out, code := cliOutput(t, "codex", "site", "init", "--out", filepath.Join(root, "blank")); code != ExitOK || !strings.Contains(out, "skeleton initialized") {
		t.Fatalf("site init: exit %d\n%s", code, out)
	}
	if out, code := cliOutput(t, "codex", "generate", "--docs", docs, "--out", site, "--tier", "internal"); code != ExitOK || !strings.Contains(out, "generated successfully") {
		t.Fatalf("generate site: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "codex", "generate", "--docs", docs, "--out", site, "--tier", "galactic"); code != ExitUsage {
		t.Fatalf("bad tier: exit %d", code)
	}
	if out, code := cliOutput(t, "codex", "site", "share", "--site", site, "--out", filepath.Join(root, "bundle.tar.gz")); code != ExitOK || !strings.Contains(out, "exported documentation sharing bundle") {
		t.Fatalf("site share: exit %d\n%s", code, out)
	}
	if _, code := cliOutput(t, "codex", "site", "share", "--site", filepath.Join(root, "missing")); code != ExitError {
		t.Fatalf("share of a missing site: exit %d", code)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if err := runCodexServe(ctx, docusaurus.NewService(), site, freePort(t), &buf); err != nil {
		t.Fatalf("serve: %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "Shutting down preview server") {
		t.Fatalf("serve output %q", buf.String())
	}
}

func TestCodex_PlaneSync(t *testing.T) {
	serveBoard(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.md"), []byte("# Page\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"push", "pull", "sync"} {
		out, _ := cliOutput(t, "codex", "plane", d, "--dir", dir)
		if !strings.Contains(out, "Synchronizing project documentation") {
			t.Errorf("plane %s: %s", d, out)
		}
	}
	if _, code := cliOutput(t, "codex", "plane", "sideways"); code != ExitUsage {
		t.Errorf("unknown direction: exit %d", code)
	}
}
