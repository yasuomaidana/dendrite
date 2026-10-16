package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewRegistryAndRegister(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}

	// Registering nil or empty command should be safe no-op
	reg.Register(nil)
	reg.Register(&Command{Name: ""})

	cmd := &Command{
		Name:        "doctor",
		Aliases:     []string{"probe", "diag", "soma-diagnose"},
		Category:    "Soma & State",
		Description: "Diagnose host environment",
		Run: func(ctx context.Context, args []string) error {
			return nil
		},
	}
	reg.Register(cmd)

	// Register second command with same category to test deduplication
	cmd2 := &Command{
		Name:        "env",
		Category:    "Soma & State",
		Description: "Replicate environments",
		Run: func(ctx context.Context, args []string) error {
			return nil
		},
	}
	reg.Register(cmd2)

	// Test primary lookup
	found, ok := reg.Lookup("doctor")
	if !ok || found.Name != "doctor" {
		t.Fatalf("expected doctor command, got %v (ok=%v)", found, ok)
	}

	// Test case insensitivity
	found, ok = reg.Lookup("DocTor")
	if !ok || found.Name != "doctor" {
		t.Fatalf("expected case-insensitive match for doctor")
	}

	// Test alias lookup
	for _, alias := range []string{"probe", "diag", "soma-diagnose"} {
		found, ok = reg.Lookup(alias)
		if !ok || found.Name != "doctor" {
			t.Fatalf("expected alias '%s' to resolve to doctor", alias)
		}
	}

	// Test non-existent command
	_, ok = reg.Lookup("nonexistent")
	if ok {
		t.Fatal("expected nonexistent command to return false")
	}

	// Test primary list
	primaries := reg.ListPrimary()
	if len(primaries) != 2 || primaries[0].Name != "doctor" {
		t.Fatalf("expected 2 primary commands, got %v", primaries)
	}

	// Test categories
	cats := reg.Categories()
	if len(cats) != 1 || cats[0] != "Soma & State" {
		t.Fatalf("expected 1 deduplicated category 'Soma & State', got %v", cats)
	}
}

func TestExecute(t *testing.T) {
	reg := NewRegistry()
	executed := false
	var capturedArgs []string

	reg.Register(&Command{
		Name:    "query",
		Aliases: []string{"arbor"},
		Run: func(ctx context.Context, args []string) error {
			executed = true
			capturedArgs = args
			return nil
		},
	})

	ctx := context.Background()

	// 1. Empty args error
	err := reg.Execute(ctx, []string{})
	if err == nil || !strings.Contains(err.Error(), "no command specified") {
		t.Fatalf("expected error on empty args, got %v", err)
	}

	// 2. Execute via primary name
	err = reg.Execute(ctx, []string{"query", "slice", "--sources", "."})
	if err != nil || !executed || len(capturedArgs) != 3 {
		t.Fatalf("failed primary execution: err=%v, args=%v", err, capturedArgs)
	}

	// 3. Execute via alias
	executed = false
	capturedArgs = nil
	err = reg.Execute(ctx, []string{"arbor", "fuse"})
	if err != nil || !executed || len(capturedArgs) != 1 || capturedArgs[0] != "fuse" {
		t.Fatalf("failed alias execution: err=%v, args=%v", err, capturedArgs)
	}

	// 4. Command without runner
	reg.Register(&Command{Name: "norun"})
	err = reg.Execute(ctx, []string{"norun"})
	if err == nil || !strings.Contains(err.Error(), "no executable runner") {
		t.Fatalf("expected no runner error, got %v", err)
	}

	// 5. Runner returning error
	reg.Register(&Command{
		Name: "fail",
		Run: func(ctx context.Context, args []string) error {
			return errors.New("boom")
		},
	})
	err = reg.Execute(ctx, []string{"fail"})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected 'boom' error, got %v", err)
	}
}

func TestSuggest(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Command{Name: "doctor", Aliases: []string{"probe"}})
	reg.Register(&Command{Name: "project", Aliases: []string{"ganglion"}})
	reg.Register(&Command{Name: "cortex", Aliases: []string{"ui"}})
	reg.Register(&Command{Name: "doc1"})
	reg.Register(&Command{Name: "doc2"})
	reg.Register(&Command{Name: "doc3"})
	reg.Register(&Command{Name: "doc4"})

	// Exact or close typo
	suggestions := reg.Suggest("doctr")
	if len(suggestions) == 0 || suggestions[0] != "doctor" {
		t.Fatalf("expected suggestion 'doctor' for 'doctr', got %v", suggestions)
	}

	// Prefix match with >3 matches to test truncation
	suggestions = reg.Suggest("doc")
	if len(suggestions) > 3 {
		t.Fatalf("expected at most 3 suggestions, got %d", len(suggestions))
	}

	// Error message formatting with suggestion
	err := reg.Execute(context.Background(), []string{"doctr"})
	if err == nil || !strings.Contains(err.Error(), "Did you mean: doctor") {
		t.Fatalf("expected 'Did you mean' message, got %v", err)
	}

	// Distant unknown command
	err = reg.Execute(context.Background(), []string{"zzzzzzzzzzzz"})
	if err == nil || !strings.Contains(err.Error(), "unknown command 'zzzzzzzzzzzz'") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestPrintUsage(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Command{
		Name:        "doctor",
		Aliases:     []string{"probe"},
		Category:    "Soma & State",
		Description: "Diagnose host environment",
	})
	reg.Register(&Command{
		Name:        "cortex",
		Aliases:     []string{"ui"},
		Category:    "Cortex & Cockpit",
		Description: "Web control interface",
	})

	var buf bytes.Buffer
	reg.PrintUsage(&buf, "Dendrite - Autonomous AI Control Plane")
	output := buf.String()

	if !strings.Contains(output, "Dendrite - Autonomous AI Control Plane") {
		t.Errorf("missing banner in usage: %s", output)
	}
	if !strings.Contains(output, "[Soma & State]") {
		t.Errorf("missing category in usage: %s", output)
	}
	if !strings.Contains(output, "doctor") || !strings.Contains(output, "probe") {
		t.Errorf("missing command or alias in usage: %s", output)
	}

	// Test PrintUsage with empty category commands
	uncatReg := NewRegistry()
	uncatReg.Register(&Command{
		Name:        "simple",
		Aliases:     []string{"s"},
		Description: "Simple command without category",
	})
	var uncatBuf bytes.Buffer
	uncatReg.PrintUsage(&uncatBuf, "")
	uncatOutput := uncatBuf.String()
	if !strings.Contains(uncatOutput, "simple") {
		t.Errorf("missing simple command in uncategorized output: %s", uncatOutput)
	}
}

func TestLevenshteinEmptyStrings(t *testing.T) {
	if d := levenshteinDistance("", "abc"); d != 3 {
		t.Errorf("expected 3, got %d", d)
	}
	if d := levenshteinDistance("abc", ""); d != 3 {
		t.Errorf("expected 3, got %d", d)
	}
	if d := levenshteinDistance("", ""); d != 0 {
		t.Errorf("expected 0, got %d", d)
	}
}
