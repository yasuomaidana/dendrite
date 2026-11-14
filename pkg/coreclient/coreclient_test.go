package coreclient

import (
	"Dendrite/pkg/coreclient/coretest"
	"context"
	"errors"
	"os/exec"
	"testing"

	"Dendrite/pkg/sidecar"
	"Dendrite/pkg/testkit"
)

func TestMain(m *testing.M) { coretest.Main(m) }

func testClient(t *testing.T) *Client {
	t.Helper()
	loc := sidecar.Locator{LookPath: func(string) (string, error) { return coretest.Binary, nil }}
	client := New(loc)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestClient_Lifecycle(t *testing.T) {
	var nilClient *Client
	if err := nilClient.Close(); err != nil {
		t.Fatalf("nil client Close err: %v", err)
	}
	emptyClient := &Client{}
	if err := emptyClient.Close(); err != nil {
		t.Fatalf("empty client Close err: %v", err)
	}

	client := testClient(t)
	if err := client.Close(); err != nil {
		t.Fatalf("active client Close err: %v", err)
	}
	// Redundant close is safe
	if err := client.Close(); err != nil {
		t.Fatalf("second Close err: %v", err)
	}
}

func TestClient_NilPoolErrors(t *testing.T) {
	var nilClient *Client
	ctx := context.Background()
	if _, err := nilClient.Complexity(ctx, "go", "func Foo() {}"); !errors.Is(err, ErrBinaryMissing) {
		t.Fatalf("expected ErrBinaryMissing on nil client, got %v", err)
	}
}

func TestDefaultAndSetDefault(t *testing.T) {
	def := Default()
	if def == nil {
		t.Fatal("expected non-nil default client")
	}

	custom := testClient(t)
	restore := SetDefault(custom)
	if got := Default(); got != custom {
		t.Fatalf("expected custom client, got %v", got)
	}
	restore()
	if got := Default(); got != def {
		t.Fatalf("expected restored default client, got %v", got)
	}
}

func TestComplexity(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	cases := map[string]struct {
		source      string
		wantSym     string
		wantCyclo   int
		expectError bool
	}{
		"test func": {
			source:    "func TestFunc() {}",
			wantSym:   "TestFunc",
			wantCyclo: 3,
		},
		"exceeded func": {
			source:    "func ExceededFunc() {}",
			wantSym:   "ExceededFunc",
			wantCyclo: 11,
		},
		"boundary func": {
			source:    "func BoundaryFunc() {}",
			wantSym:   "BoundaryFunc",
			wantCyclo: 10,
		},
		"high complexity": {
			source:    "func HighComplexity() {}",
			wantSym:   "HighComplexity",
			wantCyclo: 15,
		},
		"syntax error": {
			source:      "func broken(",
			expectError: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			metrics, err := client.Complexity(ctx, "go", tc.source)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			testkit.Check(t, err)
			if len(metrics) != 1 {
				t.Fatalf("expected 1 metric, got %d", len(metrics))
			}
			testkit.Equal(t, "symbol", metrics[0].Symbol, tc.wantSym)
			testkit.Equal(t, "cyclomatic", metrics[0].Cyclomatic, tc.wantCyclo)
		})
	}
}

func TestFoldSignatures(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	// With keep symbols
	src := `package service
func NewClient() string { return "new" }
func CallAPI() error {
	println("heavy logic")
	for i := 0; i < 3; i++ {
		println("retrying request against", c.Endpoint, "attempt", i)
		if c.Endpoint == "" {
			return nil
		}
	}
	println("request completed successfully after retries")
	return nil
}`
	folded, err := client.FoldSignatures(ctx, "go", src, []string{"NewClient"})
	testkit.Check(t, err)
	if len(folded.Folded) != 1 || folded.Folded[0] != "CallAPI" {
		t.Fatalf("expected CallAPI folded, got %v", folded.Folded)
	}

	// Nil keep symbols
	foldedNil, err := client.FoldSignatures(ctx, "go", "func (r *Runner) Step() { println(\"stepping\") }", nil)
	testkit.Check(t, err)
	if len(foldedNil.Folded) != 1 || foldedNil.Folded[0] != "Step" {
		t.Fatalf("expected Step folded, got %v", foldedNil.Folded)
	}

	// Error path
	if _, err := client.FoldSignatures(ctx, "go", "FORCE_FOLD_ERROR", nil); err == nil {
		t.Fatal("expected error on fold error, got nil")
	}
}

func TestLastCompleteNode(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	offset, err := client.LastCompleteNode(ctx, "go", "package main\nfunc A() {}")
	testkit.Check(t, err)
	testkit.Equal(t, "offset", offset, 42)

	// Incomplete node error mapping to ErrNoCompleteNode
	_, err = client.LastCompleteNode(ctx, "go", "incomplete")
	if !errors.Is(err, ErrNoCompleteNode) {
		t.Fatalf("expected ErrNoCompleteNode, got %v", err)
	}

	// General error
	if _, err := client.LastCompleteNode(ctx, "go", "FAIL"); err == nil {
		t.Fatal("expected error for FAIL buffer")
	}
}

func TestChangedSymbols(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	hunks := []Hunk{{StartLine: 1, EndLine: 5}}
	symbols, err := client.ChangedSymbols(ctx, "go", "package main\nfunc Foo() {}", hunks)
	testkit.Check(t, err)
	if len(symbols) != 1 || symbols[0].Symbol != "TargetFunc" {
		t.Fatalf("expected TargetFunc symbol, got %v", symbols)
	}

	// Nil hunks
	symbolsNil, err := client.ChangedSymbols(ctx, "go", "package main", nil)
	testkit.Check(t, err)
	if len(symbolsNil) != 1 {
		t.Fatalf("expected 1 symbol for nil hunks, got %d", len(symbolsNil))
	}

	// Error
	if _, err := client.ChangedSymbols(ctx, "go", "FAIL", nil); err == nil {
		t.Fatal("expected error for FAIL source")
	}
}

func TestCountTokens(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	res, err := client.CountTokens(ctx, "qwen", "hello world")
	testkit.Check(t, err)
	if res.Count <= 0 {
		t.Fatalf("expected positive count, got %d", res.Count)
	}
	testkit.Equal(t, "exact", res.Exact, false)

	// Error
	if _, err := client.CountTokens(ctx, "qwen", "FAIL"); err == nil {
		t.Fatal("expected error for FAIL text")
	}
}

func TestChunkID(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	hexDigest, err := client.ChunkID(ctx, "pkg/test.go", "Foo", "func Foo() {}")
	testkit.Check(t, err)
	if len(hexDigest) != 64 {
		t.Fatalf("expected 64 char hex digest, got %q", hexDigest)
	}

	// Error
	if _, err := client.ChunkID(ctx, "pkg/test.go", "Foo", "FAIL"); err == nil {
		t.Fatal("expected error for FAIL content")
	}
}

func TestHealth(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	h, err := client.Health(ctx)
	testkit.Check(t, err)
	testkit.Equal(t, "name", h.Name, "dendrite-core")
	testkit.Equal(t, "protocol", h.Protocol, "1.0.0")
}

func TestPackageLevelFunctions(t *testing.T) {
	custom := testClient(t)
	restore := SetDefault(custom)
	defer restore()

	ctx := context.Background()

	metrics, err := Complexity(ctx, "go", "func TestFunc() {}")
	testkit.Check(t, err)
	if len(metrics) == 0 {
		t.Fatal("expected non-empty metrics")
	}

	folded, err := FoldSignatures(ctx, "go", "func Pruned() { println(\"prune me\") }", nil)
	testkit.Check(t, err)
	if folded == nil {
		t.Fatal("expected folded result")
	}

	offset, err := LastCompleteNode(ctx, "go", "package main\nfunc Foo() {}")
	testkit.Check(t, err)
	testkit.Equal(t, "offset", offset, 42)

	symbols, err := ChangedSymbols(ctx, "go", "package main\nfunc Foo() {}", []Hunk{{StartLine: 1, EndLine: 2}})
	testkit.Check(t, err)
	if len(symbols) == 0 {
		t.Fatal("expected changed symbols")
	}

	tc, err := CountTokens(ctx, "qwen", "hello")
	testkit.Check(t, err)
	if tc.Count <= 0 {
		t.Fatal("expected positive token count")
	}

	hexDigest, err := ChunkID(ctx, "a.go", "A", "code")
	testkit.Check(t, err)
	if hexDigest == "" {
		t.Fatal("expected hex digest")
	}
}

func TestClient_MissingBinaryError(t *testing.T) {
	t.Setenv(sidecar.EnvVar(BinaryName), "")
	missingLoc := sidecar.Locator{
		Home:     t.TempDir(),
		UserHome: t.TempDir(),
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
	}
	client := New(missingLoc)
	defer client.Close()

	_, err := client.Complexity(context.Background(), "go", "func Foo() {}")
	if !errors.Is(err, ErrBinaryMissing) {
		t.Fatalf("expected ErrBinaryMissing, got %v", err)
	}
	if _, err := client.Health(context.Background()); !errors.Is(err, ErrBinaryMissing) {
		t.Fatalf("Health without the binary: expected ErrBinaryMissing, got %v", err)
	}
}

func TestClient_NewWithPool(t *testing.T) {
	loc := sidecar.Locator{LookPath: func(string) (string, error) { return coretest.Binary, nil }}
	pool := sidecar.NewPool(sidecar.PoolConfig{
		Binary:  BinaryName,
		Major:   Protocol,
		Locator: loc,
	})
	client := NewWithPool(pool)
	defer client.Close()

	h, err := client.Health(context.Background())
	testkit.Check(t, err)
	testkit.Equal(t, "name", h.Name, "dendrite-core")
}

func TestClient_EnvironmentBinaryOrError(t *testing.T) {
	loc := sidecar.DefaultLocator()
	ctx := context.Background()
	path, locateErr := loc.Locate(BinaryName)
	client := New(loc)
	defer client.Close()

	metrics, err := client.Complexity(ctx, "go", "func TestFunc() {}")
	if locateErr == nil {
		// Found real binary or configured env
		testkit.Check(t, err)
		if len(metrics) == 0 {
			t.Fatal("expected at least one metric from binary at " + path)
		}
	} else {
		// Not found
		if !errors.Is(err, ErrBinaryMissing) {
			t.Fatalf("expected ErrBinaryMissing when binary not found, got %v", err)
		}
	}
}

func TestComplexityBatch(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	empty, err := client.ComplexityBatch(ctx, nil)
	testkit.Check(t, err)
	if empty != nil {
		t.Fatalf("an empty batch must send nothing and answer nothing, got %v", empty)
	}

	got, err := client.ComplexityBatch(ctx, []SourceFile{
		{Lang: "go", Source: "func ExceededFunc() {}"},
		{Lang: "go", Source: "func broken("},
		{Lang: "go", Source: "func StripSpaces() {}"},
	})
	testkit.Check(t, err)
	testkit.Equal(t, "files answered", len(got), 3)
	testkit.Equal(t, "first symbol", got[0].Functions[0].Symbol, "ExceededFunc")
	testkit.Equal(t, "first cyclomatic", got[0].Functions[0].Cyclomatic, 11)
	testkit.Contains(t, got[1].Error, "syntax error")
	testkit.Equal(t, "a bad file has no functions", len(got[1].Functions), 0)
	testkit.Equal(t, "third symbol", got[2].Functions[0].Symbol, "StripSpaces")
}

func TestComplexityBatch_Failures(t *testing.T) {
	files := []SourceFile{{Lang: "go", Source: "func A() {}"}}
	for name, tc := range map[string]struct {
		mode string
		want func(error) bool
	}{
		"an old sidecar":       {"nobatch", func(err error) bool { return errors.Is(err, ErrBatchUnsupported) }},
		"a short answer":       {"shortbatch", func(err error) bool { return err != nil && !errors.Is(err, ErrBatchUnsupported) }},
		"a sidecar that exits": {"exit", func(err error) bool { return err != nil }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("TESTSIDECAR_MODE", tc.mode)
			_, err := testClient(t).ComplexityBatch(context.Background(), files)
			if !tc.want(err) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	var nilClient *Client
	if _, err := nilClient.ComplexityBatch(context.Background(), files); !errors.Is(err, ErrBinaryMissing) {
		t.Fatalf("a nil client must report ErrBinaryMissing, got %v", err)
	}
}
