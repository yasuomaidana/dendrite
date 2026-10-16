package penpot

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParser_SvelteComponent(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "Button.svelte")

	content := `<script>
  export let label = "Submit";
  export let variant = "primary";
  export let disabled = false;
</script>

<button class="bg-primary-600 text-white p-4 px-6 py-2 rounded-xl shadow-md border-gray-300 gap-2 flex hover:bg-primary-700 disabled:opacity-50">
  {label}
</button>`

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewParser()
	spec, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if spec.Name != "Button" {
		t.Errorf("expected Button, got %s", spec.Name)
	}
	if spec.Framework != "svelte" {
		t.Errorf("expected svelte, got %s", spec.Framework)
	}
	if len(spec.Props) != 3 {
		t.Errorf("expected 3 props, got %v", spec.Props)
	}
	if _, ok := spec.Tokens["bg-primary-600"]; !ok {
		t.Errorf("expected bg-primary-600 in tokens")
	}
	if len(spec.States) == 0 {
		t.Errorf("expected states, got none")
	}
}

func TestParser_Svelte5Props(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "Badge.svelte")
	content := `<script>
  let { text, active } = $props();
</script>
<span class="bg-blue-500 text-white p-2 rounded-full">{text}</span>`
	_ = os.WriteFile(filePath, []byte(content), 0644)

	p := NewParser()
	spec, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Props) != 2 {
		t.Errorf("expected 2 props, got %v", spec.Props)
	}
}

func TestParser_VueAndCSS(t *testing.T) {
	tmpDir := t.TempDir()

	vueFile := filepath.Join(tmpDir, "Modal.vue")
	_ = os.WriteFile(vueFile, []byte(`<template><div class="bg-white p-4"></div></template>`), 0644)

	cssFile := filepath.Join(tmpDir, "theme.css")
	_ = os.WriteFile(cssFile, []byte(`.btn { padding: 4px; }`), 0644)

	p := NewParser()
	specVue, err := p.ParseFile(vueFile)
	if err != nil || specVue.Framework != "vue" {
		t.Errorf("expected vue, got %v", specVue)
	}

	specCSS, err := p.ParseFile(cssFile)
	if err != nil || specCSS.Framework != "css" {
		t.Errorf("expected css, got %v", specCSS)
	}

	// Error reading file
	_, err = p.ParseFile(filepath.Join(tmpDir, "missing.svelte"))
	if err == nil {
		t.Error("expected error reading missing file")
	}
}

func TestParser_ReactComponent(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "Card.tsx")

	content := `import React from 'react';

interface CardProps {
  title: string;
  count?: number;
}

export const Card: React.FC<CardProps> = ({ title, count }) => {
  return (
    <div className="bg-white p-6 rounded-2xl shadow-lg border-gray-200 hover:shadow-xl dark:bg-gray-800">
      <h2>{title}</h2>
      <p>{count}</p>
    </div>
  );
};`

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewParser()
	spec, err := p.ParseFile(filePath)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if spec.Name != "Card" {
		t.Errorf("expected Card, got %s", spec.Name)
	}
	if spec.Framework != "react" {
		t.Errorf("expected react, got %s", spec.Framework)
	}
	if len(spec.Props) != 2 {
		t.Errorf("expected 2 props, got %v", spec.Props)
	}
}

func TestTokenDiff(t *testing.T) {
	oldTokens := map[string]string{
		"bg-primary-500": "background",
		"p-2":            "spacing",
		"shadow-sm":      "shadow",
	}
	newTokens := map[string]string{
		"bg-primary-600": "background",   // added
		"p-2":            "layout",       // changed category
	}

	diff := Diff(oldTokens, newTokens, "Button")
	if !diff.HasDrift {
		t.Errorf("expected drift")
	}
	if len(diff.AddedTokens) != 1 {
		t.Errorf("expected 1 added token, got %d", len(diff.AddedTokens))
	}
	if len(diff.ChangedTokens) != 1 {
		t.Errorf("expected 1 changed token, got %d", len(diff.ChangedTokens))
	}
	if len(diff.RemovedTokens) != 2 {
		t.Errorf("expected 1 removed token, got %d", len(diff.RemovedTokens))
	}
}

func TestSynchronizer_Sync_DryRunAndErrors(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "MetricCard.svelte")
	content := `<script>export let metric = 0;</script><div class="p-4 bg-gray-100 rounded-lg">Metric: {metric}</div>`
	_ = os.WriteFile(filePath, []byte(content), 0644)

	syncer := NewSynchronizer(nil)
	res, err := syncer.Sync(context.Background(), SyncOptions{
		TicketID:       "DEN-810",
		FilePath:       filePath,
		PenpotEndpoint: "http://localhost:9000",
		DryRun:         true,
	})

	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success")
	}
	if res.PenpotFrameURL != "http://localhost:9000/#/workspace/dendrite/project/ui?frame=metriccard" {
		t.Errorf("unexpected penpot frame URL: %s", res.PenpotFrameURL)
	}

	// Missing file error in sync
	_, err = syncer.Sync(context.Background(), SyncOptions{
		FilePath: filepath.Join(tmpDir, "nonexistent.svelte"),
	})
	if err == nil {
		t.Fatal("expected error on nonexistent file")
	}

	// Non-dryrun sync without plane comment
	liveRes, err := syncer.Sync(context.Background(), SyncOptions{
		TicketID:       "DEN-810",
		FilePath:       filePath,
		PenpotEndpoint: "http://localhost:9000",
		DryRun:         false,
		CommentPlane:   false,
	})
	if err != nil {
		t.Fatalf("live sync failed: %v", err)
	}
	if !liveRes.Success {
		t.Fatal("expected live sync success")
	}
}
