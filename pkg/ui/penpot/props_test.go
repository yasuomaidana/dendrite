package penpot

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/sidecar"
)

// parseWithCore parses a component file with the real dendrite-core sidecar. Where the sidecar is not
// installed the parse must fail with ErrBinaryMissing, and ok is false; nothing is skipped silently.
func parseWithCore(t *testing.T, file, source string) (spec *ComponentSpec, ok bool) {
	t.Helper()
	path := filepath.Join(t.TempDir(), file)
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := NewParser().ParseFile(path)
	if errors.Is(err, coreclient.ErrBinaryMissing) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", file, err)
	}
	return spec, true
}

// The props the old regexes lost or misread: object defaults with braces, renames, rest collectors,
// callback-typed members and Props types declared apart from the destructuring.
func TestParserReadsPropsFromTheSyntaxTree(t *testing.T) {
	cases := []struct {
		name, file, source string
		want               []string
	}{
		{"svelte 5 nested-brace default, rename, rest", "A.svelte", `<script>
  let { a = { x: { y: 1 } }, b: renamed, onClick = () => {}, ...rest } = $props();
</script>
<div class="p-2"></div>`, []string{"a", "b", "onClick"}},
		{"svelte 5 with a Props interface", "B.svelte", `<script lang="ts">
  interface Props { label: string; onSelect: (item: { id: number }) => void; size?: number }
  let { label, onSelect }: Props = $props();
</script>
<button>{label}</button>`, []string{"label", "onSelect", "size"}},
		{"svelte legacy export let", "C.svelte", `<script>
  export let label = "Submit";
  export let variant = "primary";
</script>`, []string{"label", "variant"}},
		{"react interface with nested braces and callbacks", "D.tsx", `interface ButtonProps {
  style?: { color: string; margin: { top: number } };
  onClick: (e: { x: number }) => void;
  label: string;
}
export function Button({ style, onClick, label }: ButtonProps) {
  return <button style={style} onClick={onClick}>{label}</button>;
}`, []string{"label", "onClick", "style"}},
		{"react without a props type", "E.jsx", `export const Tag = ({ text, tone = "info" }) => <span className={tone}>{text}</span>;`, []string{"text", "tone"}},
		{"no props", "F.svelte", `<p class="text-sm">static</p>`, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec, ok := parseWithCore(t, c.file, c.source)
			if ok && !slices.Equal(spec.Props, c.want) {
				t.Fatalf("props = %v, want %v", spec.Props, c.want)
			}
		})
	}
}

func TestParserFailsClearlyWithoutTheSidecar(t *testing.T) {
	t.Setenv(sidecar.EnvVar(coreclient.BinaryName), "")
	missing := coreclient.New(sidecar.Locator{
		Home:     t.TempDir(),
		UserHome: t.TempDir(),
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
	})
	t.Cleanup(coreclient.SetDefault(missing))
	t.Cleanup(func() { _ = missing.Close() })
	path := filepath.Join(t.TempDir(), "A.svelte")
	if err := os.WriteFile(path, []byte(`<script>let { a } = $props();</script>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewParser().ParseFile(path); !errors.Is(err, coreclient.ErrBinaryMissing) {
		t.Fatalf("want ErrBinaryMissing, got %v", err)
	}
	// A file with no grammar needs no sidecar.
	vue := filepath.Join(t.TempDir(), "M.vue")
	if err := os.WriteFile(vue, []byte(`<template><div class="p-4"></div></template>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if spec, err := NewParser().ParseFile(vue); err != nil || len(spec.Props) != 0 {
		t.Fatalf("vue = %+v, %v", spec, err)
	}
}
