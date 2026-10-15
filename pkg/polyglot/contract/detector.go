package contract

import (
	"fmt"
	"strings"
)

// CompareSchemas evaluates drift between a canonical specification and an implementation model.
func CompareSchemas(canonical ContractSchema, codeModel ContractSchema) []DriftViolation {
	var violations []DriftViolation

	for fieldTag, canField := range canonical.Fields {
		modelField, exists := codeModel.Fields[fieldTag]
		if !exists {
			// Check if field was defined by internal struct name rather than serialization tag
			foundByName := false
			for _, mf := range codeModel.Fields {
				if strings.EqualFold(mf.Name, canField.Name) {
					foundByName = true
					modelField = mf
					break
				}
			}

			if !foundByName {
				violations = append(violations, DriftViolation{
					SchemaName: canonical.Name,
					FieldName:  fieldTag,
					Type:       DriftFieldMissing,
					Severity:   "ERROR",
					TargetLang: codeModel.Language,
					TargetFile: codeModel.SourceFile,
					Message:    fmt.Sprintf("Required contract field '%s' in schema '%s' is missing in %s model", fieldTag, canonical.Name, codeModel.Language),
					Expected:   fmt.Sprintf("%s (%s)", fieldTag, canField.NormalizedType),
					Actual:     "MISSING",
					Line:       1,
				})
				continue
			}
		}

		// 1. Type Mismatch Check
		if canField.NormalizedType != "" && modelField.NormalizedType != "" {
			if !typesCompatible(canField.NormalizedType, modelField.NormalizedType) {
				violations = append(violations, DriftViolation{
					SchemaName: canonical.Name,
					FieldName:  fieldTag,
					Type:       DriftTypeMismatch,
					Severity:   "ERROR",
					TargetLang: codeModel.Language,
					TargetFile: codeModel.SourceFile,
					Message:    fmt.Sprintf("Type mismatch for field '%s' in %s model: expected '%s', got '%s'", fieldTag, codeModel.Language, canField.NormalizedType, modelField.NormalizedType),
					Expected:   canField.NormalizedType,
					Actual:     modelField.NormalizedType,
					Line:       modelField.LineNumber,
				})
			}
		}

		// 2. Serialization Tag Discrepancy Check
		if canField.SerializationTag != "" && modelField.SerializationTag != "" {
			if canField.SerializationTag != modelField.SerializationTag {
				violations = append(violations, DriftViolation{
					SchemaName: canonical.Name,
					FieldName:  fieldTag,
					Type:       DriftTagMismatch,
					Severity:   "WARNING",
					TargetLang: codeModel.Language,
					TargetFile: codeModel.SourceFile,
					Message:    fmt.Sprintf("Serialization wire tag mismatch for field '%s': contract expects '%s', code serializes as '%s'", fieldTag, canField.SerializationTag, modelField.SerializationTag),
					Expected:   canField.SerializationTag,
					Actual:     modelField.SerializationTag,
					Line:       modelField.LineNumber,
				})
			}
		}

		// 3. Optionality Check (Schema requires field, but model declares it optional)
		if !canField.Optional && modelField.Optional {
			violations = append(violations, DriftViolation{
				SchemaName: canonical.Name,
				FieldName:  fieldTag,
				Type:       DriftOptionalError,
				Severity:   "WARNING",
				TargetLang: codeModel.Language,
				TargetFile: codeModel.SourceFile,
				Message:    fmt.Sprintf("Field '%s' is required by contract '%s' but marked optional in %s model", fieldTag, canonical.Name, codeModel.Language),
				Expected:   "required",
				Actual:     "optional",
				Line:       modelField.LineNumber,
			})
		}
	}

	return violations
}

// CompareAll evaluates a set of canonical schemas against code models across languages.
func CompareAll(canonicalSchemas []ContractSchema, codeSchemas []ContractSchema) *DriftReport {
	report := &DriftReport{
		ScannedContracts:  len(canonicalSchemas),
		ScannedCodeModels: len(codeSchemas),
		Violations:        make([]DriftViolation, 0),
	}

	for _, can := range canonicalSchemas {
		for _, code := range codeSchemas {
			if strings.EqualFold(can.Name, code.Name) {
				violations := CompareSchemas(can, code)
				report.Violations = append(report.Violations, violations...)
			}
		}
	}

	report.TotalViolations = len(report.Violations)
	for _, v := range report.Violations {
		if v.Severity == "ERROR" {
			report.HasErrors = true
			break
		}
	}

	return report
}

func typesCompatible(expected, actual string) bool {
	if expected == actual {
		return true
	}
	// Allow numeric promotions if compatible
	if (expected == "int32" || expected == "int64") && (actual == "int32" || actual == "int64") {
		return true
	}
	if (expected == "float32" || expected == "float64") && (actual == "float32" || actual == "float64") {
		return true
	}
	return false
}
