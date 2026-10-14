package translate

import (
	"encoding/json"
	"errors"
)

// Standard error definitions for translation and ISR parsing.
var (
	ErrEmptySource    = errors.New("source code cannot be empty")
	ErrUnsupportedAST = errors.New("unsupported AST construct in source language")
)

// TypeKind defines normalized primitive and composite types in the ISR.
type TypeKind string

const (
	TypeInt    TypeKind = "int"
	TypeFloat  TypeKind = "float"
	TypeString TypeKind = "string"
	TypeBool   TypeKind = "bool"
	TypeList   TypeKind = "list"
	TypeMap    TypeKind = "map"
	TypeCustom TypeKind = "custom"
	TypeVoid   TypeKind = "void"
)

// ISRType encapsulates type metadata, generic parameters, and nullability.
type ISRType struct {
	Kind       TypeKind `json:"kind"`
	Name       string   `json:"name"`
	KeyType    *ISRType `json:"key_type,omitempty"`
	ValueType  *ISRType `json:"value_type,omitempty"`
	IsOptional bool     `json:"is_optional"`
}

// ISRParameter describes a function argument in the ISR model.
type ISRParameter struct {
	Name         string  `json:"name"`
	Type         ISRType `json:"type"`
	DefaultValue string  `json:"default_value,omitempty"`
}

// ISRConcurrencyKind denotes async/await or multithreading patterns.
type ISRConcurrencyKind string

const (
	ConcurrencyNone      ISRConcurrencyKind = "none"
	ConcurrencyAsync     ISRConcurrencyKind = "async"
	ConcurrencyGoroutine ISRConcurrencyKind = "goroutine"
)

// ISRFunction represents a language-agnostic function or method definition.
type ISRFunction struct {
	Name           string             `json:"name"`
	Docstring      string             `json:"docstring,omitempty"`
	Parameters     []ISRParameter     `json:"parameters"`
	ReturnType     ISRType            `json:"return_type"`
	IsFallible     bool               `json:"is_fallible"`
	ErrorType      string             `json:"error_type,omitempty"`
	Concurrency    ISRConcurrencyKind `json:"concurrency"`
	BodyStatements []string           `json:"body_statements,omitempty"`
}

// ISRModule encapsulates an entire translated or parsed file/subsystem.
type ISRModule struct {
	SourceLanguage string        `json:"source_language"`
	ModuleName     string        `json:"module_name"`
	Functions      []ISRFunction `json:"functions"`
}

// ToJSON serializes the ISRModule to formatted JSON bytes.
func (m *ISRModule) ToJSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// FromJSON deserializes JSON bytes into an ISRModule.
func FromJSON(data []byte) (*ISRModule, error) {
	var mod ISRModule
	if err := json.Unmarshal(data, &mod); err != nil {
		return nil, err
	}
	return &mod, nil
}
