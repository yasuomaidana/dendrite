package testgen

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSymbolsAndClassifyDependencies(t *testing.T) {
	sampleSource := `package example

import (
	"context"
	"database/sql"
	"net/http"
	"github.com/redis/go-redis/v9"
	"github.com/mattn/go-sqlite3"
)

type Config struct {
	Options map[string]interface{}
	Tags    []string
}

type CacheStore struct {
	db *sql.DB
	rdb *redis.Client
	httpClient *http.Client
	sqliteDriver *sqlite3.SQLiteDriver
}

func NewCacheStore(db *sql.DB) *CacheStore {
	return &CacheStore{db: db}
}

func (s *CacheStore) GetKey(ctx context.Context, key string) (string, error) {
	return "val", nil
}

func unexportedHelper() bool {
	return true
}
`
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "example.go")
	if err := os.WriteFile(srcFile, []byte(sampleSource), 0644); err != nil {
		t.Fatalf("failed to write tmp source: %v", err)
	}

	symbols, pkgName, err := ExtractSymbols(srcFile)
	if err != nil {
		t.Fatalf("ExtractSymbols failed: %v", err)
	}
	if pkgName != "example" {
		t.Errorf("expected package 'example', got %s", pkgName)
	}
	if len(symbols) == 0 {
		t.Fatalf("expected extracted symbols")
	}

	// Verify symbol properties
	foundNew := false
	foundGetKey := false
	for _, sym := range symbols {
		if sym.Name == "NewCacheStore" {
			foundNew = true
			if !sym.IsExported {
				t.Errorf("expected NewCacheStore to be exported")
			}
		}
		if sym.Name == "GetKey" {
			foundGetKey = true
			if sym.Receiver != "*CacheStore" {
				t.Errorf("expected receiver *CacheStore, got %s", sym.Receiver)
			}
		}
	}
	if !foundNew || !foundGetKey {
		t.Errorf("missing expected functions in extracted symbols: %v", symbols)
	}

	// Dependency classification
	deps := ClassifyDependencies(sampleSource)
	if len(deps) < 3 {
		t.Fatalf("expected at least 3 dependencies, got %v", deps)
	}
	hasPostgres := false
	hasRedis := false
	hasSQLite := false
	hasHTTP := false
	for _, d := range deps {
		switch d {
		case DepPostgres:
			hasPostgres = true
		case DepRedis:
			hasRedis = true
		case DepSQLite:
			hasSQLite = true
		case DepHTTPClient:
			hasHTTP = true
		}
	}
	if !hasPostgres || !hasRedis || !hasSQLite || !hasHTTP {
		t.Errorf("expected all dependencies detected, got: %v", deps)
	}
	if !RequiresTestcontainers(deps) {
		t.Errorf("expected RequiresTestcontainers to be true")
	}

	// Test no dependencies
	noDeps := ClassifyDependencies("package simple\nfunc Do() {}")
	if len(noDeps) != 1 || noDeps[0] != DepNone {
		t.Errorf("expected DepNone, got %v", noDeps)
	}
	if RequiresTestcontainers(noDeps) {
		t.Errorf("expected RequiresTestcontainers to be false for DepNone")
	}
}

func TestSynthesizeWithTestcontainers(t *testing.T) {
	symbols := []SourceSymbol{
		{
			Name:        "QueryUser",
			Kind:        "func",
			Signature:   "QueryUser(id string) error",
			IsExported:  true,
			ParamTypes:  []string{"string"},
			ReturnTypes: []string{"error"},
		},
		{
			Name:       "internalHelper",
			Kind:       "func",
			IsExported: false,
		},
		{
			Name:       "UserStore",
			Kind:       "struct",
			IsExported: true,
		},
	}

	deps := []DependencyType{DepPostgres, DepRedis}
	code := SynthesizeTest("userpkg", symbols, true, deps)
	if len(code) == 0 {
		t.Fatalf("expected generated test code")
	}
	if !isExported("ExportedFunc") || isExported("unexported") || isExported("") {
		t.Errorf("unexpected isExported behavior")
	}
}

func TestAutoRepairTestCode(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "broken_test.go")
	initialCode := `package broken
import (
	"testing"
)
func TestFoo(t *testing.T) {
	fmt.Println("foo")
}
`
	if err := os.WriteFile(testPath, []byte(initialCode), 0644); err != nil {
		t.Fatalf("failed to write test code: %v", err)
	}

	runner := NewSelfHealingRunner(3)
	// 1. Repair missing fmt
	patch1 := runner.autoRepairTestCode(testPath, "undefined: fmt")
	if patch1 != "injected missing fmt import" {
		t.Errorf("expected missing fmt patch, got %s", patch1)
	}

	// 2. Repair missing context
	patch2 := runner.autoRepairTestCode(testPath, "undefined: context")
	if patch2 != "injected missing context import" {
		t.Errorf("expected missing context patch, got %s", patch2)
	}

	// 3. Repair unused imports
	patch3 := runner.autoRepairTestCode(testPath, "\"fmt\" imported and not used")
	if patch3 != "removed unused imports" {
		t.Errorf("expected removed unused imports, got %s", patch3)
	}

	// 4. Bad path handling
	badPatch := runner.autoRepairTestCode(filepath.Join(tmpDir, "non_existent.go"), "err")
	if badPatch != "failed to read test file for repair" {
		t.Errorf("expected read failure patch description, got %s", badPatch)
	}
}

func TestSynthesizeTestAndSelfHealingRunner(t *testing.T) {
	sampleSource := `package mathutil

func Add(a, b int) int {
	return a + b
}

func Multiply(a, b int) (int, error) {
	return a * b, nil
}
`
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "mathutil.go")
	if err := os.WriteFile(srcFile, []byte(sampleSource), 0644); err != nil {
		t.Fatalf("failed to write tmp source: %v", err)
	}

	runner := NewSelfHealingRunner(3)
	cfg := TestSynthesisConfig{
		SourceFile:         srcFile,
		WithTestcontainers: false,
		TargetCoverage:     85.0,
	}

	report, err := runner.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("SelfHealingRunner.Run failed: %v", err)
	}

	if !report.Success {
		t.Fatalf("expected test synthesis success, got state: %s", report.State)
	}
	if report.Coverage < 85.0 {
		t.Errorf("expected coverage >= 85%%, got %.1f%%", report.Coverage)
	}
	if report.State != StateVerified {
		t.Errorf("expected StateVerified, got %s", report.State)
	}

	// Verify generated test file exists
	if _, statErr := os.Stat(report.TestFile); statErr != nil {
		t.Errorf("generated test file not found: %s", report.TestFile)
	}

	// Test error cases
	badCfg := TestSynthesisConfig{SourceFile: ""}
	_, badErr := runner.Run(context.Background(), badCfg)
	if badErr == nil {
		t.Errorf("expected error for empty source_file")
	}

	nonExistentCfg := TestSynthesisConfig{SourceFile: filepath.Join(tmpDir, "non_existent.go")}
	_, nonExistentErr := runner.Run(context.Background(), nonExistentCfg)
	if nonExistentErr == nil {
		t.Errorf("expected error for non-existent file")
	}

	// Test invalid syntax in source file
	badSyntaxFile := filepath.Join(tmpDir, "bad.go")
	_ = os.WriteFile(badSyntaxFile, []byte("package bad\nfunc ( {"), 0644)
	badSyntaxCfg := TestSynthesisConfig{SourceFile: badSyntaxFile}
	_, badSyntaxErr := runner.Run(context.Background(), badSyntaxCfg)
	if badSyntaxErr == nil {
		t.Errorf("expected error for invalid AST source")
	}
}
