package verify_test

import (
	"strings"
	"testing"

	"Dendrite/pkg/ai/translate"
	"Dendrite/pkg/ai/translate/verify"
)

func TestSynthesizeGoFromISR(t *testing.T) {
	// 1. Error on nil module
	if _, err := verify.SynthesizeGoFromISR(nil, "sample"); err == nil {
		t.Errorf("expected error on nil module")
	}

	// 2. Synthesize module with fallible and async functions
	mod := &translate.ISRModule{
		SourceLanguage: "python",
		ModuleName:     "algorithms",
		Functions: []translate.ISRFunction{
			{
				Name:      "divide",
				Docstring: "performs division",
				Parameters: []translate.ISRParameter{
					{Name: "a", Type: translate.ISRType{Kind: translate.TypeFloat, Name: "float"}},
					{Name: "b", Type: translate.ISRType{Kind: translate.TypeFloat, Name: "float"}},
				},
				ReturnType: translate.ISRType{Kind: translate.TypeFloat, Name: "float"},
				IsFallible: true,
				ErrorType:  "ZeroDivisionError",
			},
			{
				Name:        "fetch_async",
				Parameters:  []translate.ISRParameter{{Name: "url", Type: translate.ISRType{Kind: translate.TypeString, Name: "string"}}},
				ReturnType:  translate.ISRType{Kind: translate.TypeString, Name: "string"},
				Concurrency: translate.ConcurrencyAsync,
			},
			{
				Name:       "log_action",
				Parameters: []translate.ISRParameter{{Name: "msg", Type: translate.ISRType{Kind: translate.TypeString, Name: "string"}}},
				ReturnType: translate.ISRType{Kind: translate.TypeVoid, Name: "void"},
				IsFallible: true,
				ErrorType:  "IOError",
			},
			{
				Name:       "noop",
				Parameters: []translate.ISRParameter{},
				ReturnType: translate.ISRType{Kind: translate.TypeVoid, Name: "void"},
			},
		},
	}

	// Empty targetPkg defaults to "translated"
	goSrc, err := verify.SynthesizeGoFromISR(mod, "")
	if err != nil {
		t.Fatalf("unexpected error synthesizing Go: %v", err)
	}

	// Verify package declaration
	if !strings.Contains(goSrc, "package translated") {
		t.Errorf("expected package translated in output")
	}

	// Verify imports
	if !strings.Contains(goSrc, "\"context\"") || !strings.Contains(goSrc, "\"fmt\"") {
		t.Errorf("expected context and fmt imports")
	}

	// Verify fallible signature (float64, error)
	if !strings.Contains(goSrc, "func divide(a float64, b float64) (float64, error)") {
		t.Errorf("expected (float64, error) signature for fallible function")
	}

	// Verify async function includes ctx context.Context
	if !strings.Contains(goSrc, "func fetch_async(ctx context.Context, url string) string") {
		t.Errorf("expected ctx context.Context parameter for async function")
	}

	// Verify void fallible function returns error
	if !strings.Contains(goSrc, "func log_action(msg string) error") {
		t.Errorf("expected func log_action(msg string) error")
	}

	// Verify void non-fallible function
	if !strings.Contains(goSrc, "func noop()") {
		t.Errorf("expected func noop()")
	}
}

func TestGenerateDualPropertyTest(t *testing.T) {
	// 1. Error on nil module
	if _, err := verify.GenerateDualPropertyTest(nil, "sample", 500); err == nil {
		t.Errorf("expected error on nil module")
	}

	mod := &translate.ISRModule{
		Functions: []translate.ISRFunction{
			{
				Name: "ProcessAll",
				Parameters: []translate.ISRParameter{
					{Name: "intVal", Type: translate.ISRType{Kind: translate.TypeInt}},
					{Name: "floatVal", Type: translate.ISRType{Kind: translate.TypeFloat}},
					{Name: "strVal", Type: translate.ISRType{Kind: translate.TypeString}},
					{Name: "boolVal", Type: translate.ISRType{Kind: translate.TypeBool}},
				},
				ReturnType: translate.ISRType{Kind: translate.TypeInt},
			},
		},
	}

	// Empty targetPkg defaults to "translated", clamp iterations to >= 1000
	testCode, err := verify.GenerateDualPropertyTest(mod, "", 500)
	if err != nil {
		t.Fatalf("unexpected error generating property test: %v", err)
	}

	if !strings.Contains(testCode, "package translated_test") {
		t.Errorf("expected package translated_test")
	}
	if !strings.Contains(testCode, "for i := 0; i < 1000; i++") {
		t.Errorf("expected iterations to be clamped to 1,000")
	}
	if !strings.Contains(testCode, "TestPropertyParity_ProcessAll") {
		t.Errorf("expected TestPropertyParity_ProcessAll test function")
	}
}
