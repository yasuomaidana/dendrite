package translate_test

import (
	"errors"
	"testing"

	"Dendrite/pkg/ai/translate"
)

func TestParsePythonToISR(t *testing.T) {
	// 1. Error on empty source
	if _, err := translate.ParsePythonToISR(""); !errors.Is(err, translate.ErrEmptySource) {
		t.Errorf("expected ErrEmptySource for empty python code")
	}

	// 2. Python source with synchronous and async functions, exceptions, and type hints
	pyCode := `
def calculate_metrics(count: int, rate: float) -> bool:
    """Calculates operational rates"""
    if count < 0:
        raise ValueError("count cannot be negative")
    return rate > 0.5

async def fetch_remote_data(endpoint: str) -> str:
    # Async network call
    return "response"
`

	mod, err := translate.ParsePythonToISR(pyCode)
	if err != nil {
		t.Fatalf("unexpected error parsing Python code: %v", err)
	}

	if len(mod.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(mod.Functions))
	}

	// Verify calculate_metrics
	fn1 := mod.Functions[0]
	if fn1.Name != "calculate_metrics" {
		t.Errorf("expected calculate_metrics, got %s", fn1.Name)
	}
	if len(fn1.Parameters) != 2 || fn1.Parameters[0].Name != "count" || fn1.Parameters[0].Type.Kind != translate.TypeInt {
		t.Errorf("expected count parameter of type int")
	}
	if fn1.ReturnType.Kind != translate.TypeBool {
		t.Errorf("expected return type bool, got %v", fn1.ReturnType)
	}
	if !fn1.IsFallible || fn1.ErrorType != "ValueError" {
		t.Errorf("expected fallible with ValueError, got fallible=%v, err=%s", fn1.IsFallible, fn1.ErrorType)
	}

	// Verify fetch_remote_data
	fn2 := mod.Functions[1]
	if fn2.Name != "fetch_remote_data" {
		t.Errorf("expected fetch_remote_data, got %s", fn2.Name)
	}
	if fn2.Concurrency != translate.ConcurrencyAsync {
		t.Errorf("expected ConcurrencyAsync for async def, got %v", fn2.Concurrency)
	}
}

func TestParseTypeScriptToISR(t *testing.T) {
	// 1. Error on empty source
	if _, err := translate.ParseTypeScriptToISR(""); !errors.Is(err, translate.ErrEmptySource) {
		t.Errorf("expected ErrEmptySource for empty TS code")
	}

	// 2. TypeScript code with async function and Promise unwrap
	tsCode := `
export async function processPayment(amount: number, currency: string): Promise<boolean> {
    if (amount <= 0) {
        throw new Error("Invalid amount");
    }
    return true;
}
`

	mod, err := translate.ParseTypeScriptToISR(tsCode)
	if err != nil {
		t.Fatalf("unexpected error parsing TypeScript: %v", err)
	}

	if len(mod.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(mod.Functions))
	}

	fn := mod.Functions[0]
	if fn.Name != "processPayment" {
		t.Errorf("expected processPayment, got %s", fn.Name)
	}
	if fn.Concurrency != translate.ConcurrencyAsync {
		t.Errorf("expected ConcurrencyAsync for async function")
	}
	if fn.ReturnType.Kind != translate.TypeBool {
		t.Errorf("expected Promise<boolean> to unwrap to TypeBool, got %v", fn.ReturnType)
	}
	if !fn.IsFallible {
		t.Errorf("expected throw to mark function as fallible")
	}
}

func TestISRModule_JSONRoundtrip(t *testing.T) {
	mod := &translate.ISRModule{
		SourceLanguage: "python",
		ModuleName:     "utils",
		Functions: []translate.ISRFunction{
			{
				Name: "add",
				Parameters: []translate.ISRParameter{
					{Name: "a", Type: translate.ISRType{Kind: translate.TypeInt, Name: "int"}},
					{Name: "b", Type: translate.ISRType{Kind: translate.TypeInt, Name: "int"}},
				},
				ReturnType: translate.ISRType{Kind: translate.TypeInt, Name: "int"},
			},
		},
	}

	data, err := mod.ToJSON()
	if err != nil {
		t.Fatalf("unexpected error marshalling ISR JSON: %v", err)
	}

	loaded, err := translate.FromJSON(data)
	if err != nil {
		t.Fatalf("unexpected error unmarshalling ISR JSON: %v", err)
	}

	if loaded.ModuleName != "utils" || len(loaded.Functions) != 1 {
		t.Errorf("unexpected restored ISR module: %v", loaded)
	}

	// Invalid JSON
	if _, err := translate.FromJSON([]byte("{invalid-json")); err == nil {
		t.Errorf("expected error on invalid JSON")
	}
}
