package contract

import (
	"errors"
)

var (
	// ErrNoSchemasFound is returned when schema directory contains no supported contract files.
	ErrNoSchemasFound = errors.New("no protobuf, openapi, or json-schema definitions found")
	// ErrUnsupportedSchema is returned for unsupported file extensions.
	ErrUnsupportedSchema = errors.New("unsupported schema specification format")
	// ErrInvalidSchemaContent is returned when schema parsing fails.
	ErrInvalidSchemaContent = errors.New("invalid or malformed schema content")
)

// ContractType identifies the formal specification format.
type ContractType string

const (
	ContractProtobuf   ContractType = "protobuf"
	ContractOpenAPI    ContractType = "openapi"
	ContractJSONSchema ContractType = "jsonschema"
)

// TargetLanguage identifies the ecosystem language / IDE.
type TargetLanguage string

const (
	LangGo         TargetLanguage = "go"         // GoLand
	LangRust       TargetLanguage = "rust"       // RustRover
	LangTypeScript TargetLanguage = "typescript" // WebStorm
	LangPython     TargetLanguage = "python"     // PyCharm
)

// DriftType categorizes the nature of the interface discrepancy.
type DriftType string

const (
	DriftFieldMissing  DriftType = "FIELD_MISSING"
	DriftTypeMismatch  DriftType = "TYPE_MISMATCH"
	DriftTagMismatch   DriftType = "SERIALIZATION_TAG_MISMATCH"
	DriftOptionalError DriftType = "OPTIONALITY_MISMATCH"
)

// FieldDefinition represents a single normalized property in a schema or code model.
type FieldDefinition struct {
	Name             string `json:"name"`
	NormalizedType   string `json:"normalized_type"` // string, int32, int64, float32, float64, bool, bytes, array, map
	RawType          string `json:"raw_type"`
	Optional         bool   `json:"optional"`
	SerializationTag string `json:"serialization_tag,omitempty"` // json key, serde rename, etc.
	LineNumber       int    `json:"line_number,omitempty"`
}

// ContractSchema holds the normalized fields of a message or model.
type ContractSchema struct {
	Name       string                     `json:"name"`
	Type       ContractType               `json:"type,omitempty"`
	SourceFile string                     `json:"source_file,omitempty"`
	Language   TargetLanguage             `json:"language,omitempty"`
	Fields     map[string]FieldDefinition `json:"fields"`
}

// DriftViolation records a single contract deviation between specification and implementation.
type DriftViolation struct {
	SchemaName string         `json:"schema_name"`
	FieldName  string         `json:"field_name"`
	Type       DriftType      `json:"type"`
	Severity   string         `json:"severity"` // ERROR, WARNING
	TargetLang TargetLanguage `json:"target_lang"`
	TargetFile string         `json:"target_file"`
	Message    string         `json:"message"`
	Expected   string         `json:"expected"`
	Actual     string         `json:"actual"`
	Line       int            `json:"line,omitempty"`
}

// DriftReport aggregates all detected discrepancies across Quad-IDE models.
type DriftReport struct {
	TotalViolations   int              `json:"total_violations"`
	HasErrors         bool             `json:"has_errors"`
	Violations        []DriftViolation `json:"violations"`
	ScannedContracts  int              `json:"scanned_contracts"`
	ScannedCodeModels int              `json:"scanned_code_models"`
}
