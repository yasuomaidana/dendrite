package contract_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"Dendrite/pkg/jetbrains"
	"Dendrite/pkg/polyglot/contract"
)

func TestNormalizeType(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"int", "int32"},
		{"int32", "int32"},
		{"i32", "int32"},
		{"integer", "int32"},
		{"int64", "int64"},
		{"i64", "int64"},
		{"long", "int64"},
		{"float", "float32"},
		{"float32", "float32"},
		{"f32", "float32"},
		{"double", "float64"},
		{"float64", "float64"},
		{"f64", "float64"},
		{"number", "float64"},
		{"string", "string"},
		{"str", "string"},
		{"text", "string"},
		{"bool", "bool"},
		{"boolean", "bool"},
		{"bytes", "bytes"},
		{"byte[]", "bytes"},
		{"vec<u8>", "bytes"},
		{"repeated string", "array"},
		{"[]string", "array"},
		{"vec<i32>", "array"},
		{"list[str]", "array"},
		{"array<string>", "array"},
		{"string[]", "array"},
		{"map<string, int>", "map"},
		{"map[string]int", "map"},
		{"dict[str, int]", "map"},
		{"hashmap<String, i32>", "map"},
		{"Option<String>", "string"},
		{"Optional[int]", "int32"},
		{"*string", "string"},
		{"CustomType", "customtype"},
	}

	for _, c := range cases {
		got := contract.NormalizeType(c.input)
		if got != c.want {
			t.Errorf("NormalizeType(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestParseProtobuf(t *testing.T) {
	protoContent := `
syntax = "proto3";

// User account message
message User {
  string user_id = 1;
  int32 age = 2;
  repeated string roles = 3;
  optional string email = 4;
}

message Order {
  int64 order_id = 1;
}
`
	schemas, err := contract.ParseProtobuf(protoContent, "user.proto")
	if err != nil {
		t.Fatalf("unexpected error parsing protobuf: %v", err)
	}
	if len(schemas) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(schemas))
	}

	user := schemas[0]
	if user.Name != "User" || len(user.Fields) != 4 {
		t.Errorf("unexpected User schema: %+v", user)
	}
	if user.Fields["roles"].NormalizedType != "array" {
		t.Errorf("expected roles to be array, got %s", user.Fields["roles"].NormalizedType)
	}
	if !user.Fields["email"].Optional {
		t.Errorf("expected email to be optional")
	}
}

func TestParseJSONSchema(t *testing.T) {
	validJSON := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"title": "UserProfile",
		"type": "object",
		"required": ["id", "username"],
		"properties": {
			"id": { "type": "string" },
			"username": { "type": "string" },
			"bio": { "type": "string" }
		}
	}`

	schemas, err := contract.ParseJSONSchema(validJSON, "user.json")
	if err != nil {
		t.Fatalf("unexpected error parsing json schema: %v", err)
	}
	if len(schemas) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(schemas))
	}

	up := schemas[0]
	if up.Name != "UserProfile" || len(up.Fields) != 3 {
		t.Errorf("unexpected schema: %+v", up)
	}
	if up.Fields["bio"].Optional != true {
		t.Errorf("expected bio to be optional")
	}
	if up.Fields["id"].Optional != false {
		t.Errorf("expected id to be required")
	}

	// Invalid JSON
	_, err = contract.ParseJSONSchema("{bad-json", "bad.json")
	if !errors.Is(err, contract.ErrInvalidSchemaContent) {
		t.Errorf("expected ErrInvalidSchemaContent on malformed json, got %v", err)
	}

	// Schema without properties
	emptyProps, _ := contract.ParseJSONSchema(`{"title": "Empty"}`, "empty.json")
	if len(emptyProps) != 0 {
		t.Errorf("expected 0 schemas for missing properties")
	}
}

func TestParseOpenAPI(t *testing.T) {
	yamlContent := `
openapi: 3.0.0
info:
  title: Sample API
  version: 0.1.0
components:
  schemas:
    Product:
      type: object
      required:
        - sku
      properties:
        sku:
          type: string
        price:
          type: number
        stock:
          type: integer
          format: int64
`
	schemas, err := contract.ParseOpenAPI(yamlContent, "api.yaml")
	if err != nil {
		t.Fatalf("unexpected error parsing openapi: %v", err)
	}
	if len(schemas) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(schemas))
	}

	prod := schemas[0]
	if prod.Name != "Product" || len(prod.Fields) != 3 {
		t.Errorf("unexpected Product schema: %+v", prod)
	}
	if prod.Fields["stock"].NormalizedType != "int64" {
		t.Errorf("expected stock int64, got %s", prod.Fields["stock"].NormalizedType)
	}
	if prod.Fields["price"].NormalizedType != "float64" {
		t.Errorf("expected price float64, got %s", prod.Fields["price"].NormalizedType)
	}

	// Invalid YAML
	_, err = contract.ParseOpenAPI(": bad yaml [", "bad.yaml")
	if !errors.Is(err, contract.ErrInvalidSchemaContent) {
		t.Errorf("expected ErrInvalidSchemaContent, got %v", err)
	}
}

func TestParseCodeModels(t *testing.T) {
	// 1. Go Struct
	goCode := `
package models

// User account
type User struct {
	ID    string   ` + "`json:\"user_id\"`" + `
	Age   int      ` + "`json:\"age\"`" + `
	Roles []string ` + "`json:\"roles\"`" + `
	Email *string  ` + "`json:\"email,omitempty\"`" + `
}
`
	goSchemas, err := contract.ParseGoStructs(goCode, "user.go")
	if err != nil || len(goSchemas) != 1 {
		t.Fatalf("failed parsing Go struct: %v, schemas=%+v", err, goSchemas)
	}
	if goSchemas[0].Fields["user_id"].NormalizedType != "string" {
		t.Errorf("unexpected Go user_id type")
	}
	if !goSchemas[0].Fields["email"].Optional {
		t.Errorf("expected pointer/omitempty to be optional")
	}

	// 2. Rust Struct
	rsCode := `
#[derive(Serialize, Deserialize)]
pub struct User {
    #[serde(rename = "user_id")]
    pub id: String,
    pub age: i32,
    pub roles: Vec<String>,
    pub email: Option<String>,
}
`
	rsSchemas, err := contract.ParseRustSerdeStructs(rsCode, "user.rs")
	if err != nil || len(rsSchemas) != 1 {
		t.Fatalf("failed parsing Rust struct: %v", err)
	}
	if rsSchemas[0].Fields["user_id"].SerializationTag != "user_id" {
		t.Errorf("expected serde rename 'user_id', got %s", rsSchemas[0].Fields["user_id"].SerializationTag)
	}
	if !rsSchemas[0].Fields["email"].Optional {
		t.Errorf("expected Option<String> to be optional")
	}

	// 3. TypeScript Interface
	tsCode := `
export interface User {
  user_id: string;
  age: number;
  roles: string[];
  email?: string;
}
`
	tsSchemas, err := contract.ParseTypeScriptInterfaces(tsCode, "user.ts")
	if err != nil || len(tsSchemas) != 1 {
		t.Fatalf("failed parsing TS interface: %v", err)
	}
	if tsSchemas[0].Fields["age"].NormalizedType != "float64" {
		t.Errorf("expected number to normalize to float64")
	}
	if !tsSchemas[0].Fields["email"].Optional {
		t.Errorf("expected ? to be optional")
	}

	// 4. Python Pydantic Model
	pyCode := `
from pydantic import BaseModel, Field
from typing import Optional, List

class User(BaseModel):
    id: str = Field(alias="user_id")
    age: int
    roles: List[str]
    email: Optional[str]
`
	pySchemas, err := contract.ParsePythonPydanticModels(pyCode, "user.py")
	if err != nil || len(pySchemas) != 1 {
		t.Fatalf("failed parsing Python model: %v", err)
	}
	if pySchemas[0].Fields["user_id"].SerializationTag != "user_id" {
		t.Errorf("expected alias 'user_id', got %s", pySchemas[0].Fields["user_id"].SerializationTag)
	}
	if !pySchemas[0].Fields["email"].Optional {
		t.Errorf("expected Optional[str] to be optional")
	}
}

func TestDriftDetection(t *testing.T) {
	canonical := contract.ContractSchema{
		Name: "User",
		Fields: map[string]contract.FieldDefinition{
			"user_id": {Name: "user_id", NormalizedType: "string", SerializationTag: "user_id", Optional: false},
			"age":     {Name: "age", NormalizedType: "int32", SerializationTag: "age", Optional: false},
			"email":   {Name: "email", NormalizedType: "string", SerializationTag: "email", Optional: true},
		},
	}

	// Model with:
	// 1. Missing field 'email'
	// 2. Type mismatch for 'age' (string instead of int32)
	// 3. Tag mismatch for 'user_id' (serializes as 'userId')
	driftingModel := contract.ContractSchema{
		Name:     "User",
		Language: contract.LangRust,
		Fields: map[string]contract.FieldDefinition{
			"user_id": {Name: "id", NormalizedType: "string", SerializationTag: "userId", Optional: false},
			"age":     {Name: "age", NormalizedType: "string", SerializationTag: "age", Optional: false},
		},
	}

	violations := contract.CompareSchemas(canonical, driftingModel)
	if len(violations) < 3 {
		t.Fatalf("expected at least 3 violations, got %d: %+v", len(violations), violations)
	}

	vTypes := make(map[contract.DriftType]bool)
	for _, v := range violations {
		vTypes[v.Type] = true
	}

	if !vTypes[contract.DriftFieldMissing] {
		t.Errorf("missing expected DriftFieldMissing")
	}
	if !vTypes[contract.DriftTypeMismatch] {
		t.Errorf("missing expected DriftTypeMismatch")
	}
	if !vTypes[contract.DriftTagMismatch] {
		t.Errorf("missing expected DriftTagMismatch")
	}

	report := contract.CompareAll([]contract.ContractSchema{canonical}, []contract.ContractSchema{driftingModel})
	if !report.HasErrors || report.TotalViolations == 0 {
		t.Errorf("expected report to register errors")
	}
}

func TestContractMonitor_EndToEnd(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "monitor_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	schemaDir := filepath.Join(tmpDir, "proto")
	goDir := filepath.Join(tmpDir, "backend")
	_ = os.MkdirAll(schemaDir, 0755)
	_ = os.MkdirAll(goDir, 0755)

	protoFile := filepath.Join(schemaDir, "service.proto")
	protoData := `message Account { string account_id = 1; int64 balance = 2; }`
	_ = os.WriteFile(protoFile, []byte(protoData), 0644)

	goFile := filepath.Join(goDir, "account.go")
	goData := "package main\ntype Account struct {\n\tAccountID string `json:\"account_id\"`\n\tBalance int64 `json:\"balance\"`\n}\n"
	_ = os.WriteFile(goFile, []byte(goData), 0644)

	monitor := contract.NewContractMonitor(nil)

	// Clean match
	report, err := monitor.CheckDrift([]string{schemaDir}, map[contract.TargetLanguage]string{
		contract.LangGo: goDir,
	})
	if err != nil {
		t.Fatalf("check drift failed: %v", err)
	}
	if report.TotalViolations != 0 {
		t.Errorf("expected 0 violations for clean match, got %d: %+v", report.TotalViolations, report.Violations)
	}

	// Missing schemas error
	_, err = monitor.ScanSchemas([]string{filepath.Join(tmpDir, "missing")})
	if !errors.Is(err, contract.ErrNoSchemasFound) {
		t.Errorf("expected ErrNoSchemasFound, got %v", err)
	}

	// Test NotifyIDE with nil bridge
	if err := monitor.NotifyIDE(context.Background(), report); err != nil {
		t.Errorf("unexpected error on nil bridge notify: %v", err)
	}

	// Test NotifyIDE with mock bridge
	fakeClient, _ := jetbrains.NewClient(nil)
	fakeBridge := jetbrains.NewBridge(fakeClient)
	fakeMonitor := contract.NewContractMonitor(fakeBridge)
	_ = fakeMonitor.NotifyIDE(context.Background(), &contract.DriftReport{TotalViolations: 1})
}

func TestContractMonitor_QuadLanguageScan(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "monitor_quad_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	schemaDir := filepath.Join(tmpDir, "schemas")
	_ = os.MkdirAll(schemaDir, 0755)

	// Add JSON schema
	jsonSchema := filepath.Join(schemaDir, "item.json")
	_ = os.WriteFile(jsonSchema, []byte(`{"title":"Item","type":"object","properties":{"id":{"type":"string"}}}`), 0644)

	// Add OpenAPI YAML
	yamlSchema := filepath.Join(schemaDir, "api.yaml")
	_ = os.WriteFile(yamlSchema, []byte("openapi: 3.0.0\ncomponents:\n  schemas:\n    Widget:\n      type: object\n      properties:\n        name:\n          type: string\n"), 0644)

	// Add OpenAPI JSON
	openAPIJson := filepath.Join(schemaDir, "openapi.json")
	_ = os.WriteFile(openAPIJson, []byte(`{"openapi":"3.0.0","components":{"schemas":{"Gadget":{"type":"object","properties":{"cost":{"type":"number"}}}}}}`), 0644)

	// Create directories for all 4 IDE languages
	rustDir := filepath.Join(tmpDir, "rust")
	tsDir := filepath.Join(tmpDir, "ts")
	pyDir := filepath.Join(tmpDir, "python")
	_ = os.MkdirAll(rustDir, 0755)
	_ = os.MkdirAll(tsDir, 0755)
	_ = os.MkdirAll(pyDir, 0755)

	_ = os.WriteFile(filepath.Join(rustDir, "item.rs"), []byte("pub struct Item {\n    pub id: String,\n}"), 0644)
	_ = os.WriteFile(filepath.Join(tsDir, "widget.ts"), []byte("export interface Widget {\n    name: string;\n}"), 0644)
	_ = os.WriteFile(filepath.Join(pyDir, "gadget.py"), []byte("class Gadget(BaseModel):\n    cost: float\n"), 0644)

	monitor := contract.NewContractMonitor(nil)

	// Scan all schemas
	schemas, err := monitor.ScanSchemas([]string{schemaDir})
	if err != nil {
		t.Fatalf("ScanSchemas failed: %v", err)
	}
	if len(schemas) < 3 {
		t.Errorf("expected at least 3 schemas, got %d", len(schemas))
	}

	// Scan all code models
	models, err := monitor.ScanCodeModels(map[contract.TargetLanguage]string{
		contract.LangRust:       rustDir,
		contract.LangTypeScript: tsDir,
		contract.LangPython:     pyDir,
	})
	if err != nil {
		t.Fatalf("ScanCodeModels failed: %v", err)
	}
	if len(models) < 3 {
		t.Errorf("expected at least 3 code models, got %d", len(models))
	}

	report, err := monitor.CheckDrift([]string{schemaDir}, map[contract.TargetLanguage]string{
		contract.LangRust:       rustDir,
		contract.LangTypeScript: tsDir,
		contract.LangPython:     pyDir,
	})
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}
	if report.TotalViolations != 0 {
		for _, v := range report.Violations { t.Logf("Violation: %+v", v) }; t.Errorf("expected 0 violations across clean quad-language models, got %d", report.TotalViolations)
	}

	// Test optional mismatch
	can := contract.ContractSchema{
		Name: "Req",
		Fields: map[string]contract.FieldDefinition{
			"f": {Name: "f", NormalizedType: "string", Optional: false},
		},
	}
	opt := contract.ContractSchema{
		Name: "Req",
		Fields: map[string]contract.FieldDefinition{
			"f": {Name: "f", NormalizedType: "string", Optional: true},
		},
	}
	vList := contract.CompareSchemas(can, opt)
	if len(vList) != 1 || vList[0].Type != contract.DriftOptionalError {
		t.Errorf("expected DriftOptionalError, got %+v", vList)
	}
}

func TestContractMonitor_NotifyIDEWithMockTransport(t *testing.T) {
	mock := jetbrains.NewMockTransport()
	mock.RegisterHandler("execute_terminal_command", func(req *jetbrains.JSONRPCRequest) (*jetbrains.JSONRPCResponse, error) {
		return &jetbrains.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: []byte(`{"output": "ok"}`)}, nil
	})
	client, err := jetbrains.NewClient(mock)
	if err != nil {
		t.Fatalf("failed creating client: %v", err)
	}
	_, _ = client.Connect(context.Background())
	bridge := jetbrains.NewBridge(client)
	monitor := contract.NewContractMonitor(bridge)

	report := &contract.DriftReport{
		TotalViolations: 2,
		HasErrors:       true,
		Violations: []contract.DriftViolation{
			{SchemaName: "User", FieldName: "id", Message: "missing"},
		},
	}

	if err := monitor.NotifyIDE(context.Background(), report); err != nil {
		t.Errorf("expected clean NotifyIDE with mock transport: %v", err)
	}
}
