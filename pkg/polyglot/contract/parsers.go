package contract

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// NormalizeType standardizes type representations across Protobuf, OpenAPI, Go, Rust, TS, and Python.
func NormalizeType(raw string) string {
	cleaned := strings.ToLower(strings.TrimSpace(raw))
	cleaned = strings.TrimPrefix(cleaned, "*") // Pointer in Go

	// Bytes check
	if cleaned == "bytes" || cleaned == "byte[]" || cleaned == "vec<u8>" {
		return "bytes"
	}

	// Array / Repeated / List checks
	if strings.HasPrefix(cleaned, "repeated ") ||
		strings.HasPrefix(cleaned, "[]") ||
		strings.HasPrefix(cleaned, "vec<") ||
		strings.HasPrefix(cleaned, "list[") ||
		strings.HasPrefix(cleaned, "array<") ||
		strings.HasSuffix(cleaned, "[]") {
		return "array"
	}

	// Map / Dictionary checks
	if strings.HasPrefix(cleaned, "map<") ||
		strings.HasPrefix(cleaned, "map[") ||
		strings.HasPrefix(cleaned, "dict[") ||
		strings.HasPrefix(cleaned, "hashmap<") {
		return "map"
	}

	// Optional wrapper checks
	if strings.HasPrefix(cleaned, "option<") && strings.HasSuffix(cleaned, ">") {
		inner := cleaned[7 : len(cleaned)-1]
		return NormalizeType(inner)
	}
	if strings.HasPrefix(cleaned, "optional[") && strings.HasSuffix(cleaned, "]") {
		inner := cleaned[9 : len(cleaned)-1]
		return NormalizeType(inner)
	}

	switch cleaned {
	case "int", "int32", "i32", "integer":
		return "int32"
	case "int64", "i64", "long":
		return "int64"
	case "float", "float32", "f32":
		return "float32"
	case "double", "float64", "f64", "number":
		return "float64"
	case "string", "str", "text":
		return "string"
	case "bool", "boolean":
		return "bool"
	case "bytes", "byte[]", "vec<u8>":
		return "bytes"
	default:
		return cleaned
	}
}

// ParseProtobuf extracts messages and field definitions from .proto files.
func ParseProtobuf(content, filename string) ([]ContractSchema, error) {
	lines := strings.Split(content, "\n")
	var schemas []ContractSchema
	var currentSchema *ContractSchema

	msgRegex := regexp.MustCompile(`^\s*message\s+([A-Za-z0-9_]+)\s*\{`)
	fieldRegex := regexp.MustCompile(`^\s*(optional\s+|repeated\s+)?([A-Za-z0-9_<>]+)\s+([A-Za-z0-9_]+)\s*=\s*\d+;`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		if matches := msgRegex.FindStringSubmatch(line); len(matches) > 1 {
			if currentSchema != nil {
				schemas = append(schemas, *currentSchema)
			}
			currentSchema = &ContractSchema{
				Name:       matches[1],
				Type:       ContractProtobuf,
				SourceFile: filename,
				Fields:     make(map[string]FieldDefinition),
			}
			continue
		}

		if currentSchema != nil && trimmed == "}" {
			schemas = append(schemas, *currentSchema)
			currentSchema = nil
			continue
		}

		if currentSchema != nil {
			if fMatches := fieldRegex.FindStringSubmatch(line); len(fMatches) > 3 {
				modifier := strings.TrimSpace(fMatches[1])
				rawType := fMatches[2]
				fieldName := fMatches[3]

				isOptional := modifier == "optional"
				if modifier == "repeated" {
					rawType = "repeated " + rawType
				}

				currentSchema.Fields[fieldName] = FieldDefinition{
					Name:             fieldName,
					NormalizedType:   NormalizeType(rawType),
					RawType:          rawType,
					Optional:         isOptional,
					SerializationTag: fieldName,
					LineNumber:       i + 1,
				}
			}
		}
	}

	if currentSchema != nil {
		schemas = append(schemas, *currentSchema)
	}

	return schemas, nil
}

// ParseJSONSchema extracts object schemas from JSON Schema definitions.
func ParseJSONSchema(content, filename string) ([]ContractSchema, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSchemaContent, err)
	}

	title, _ := raw["title"].(string)
	if title == "" {
		title = "RootSchema"
	}

	props, ok := raw["properties"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	reqMap := make(map[string]bool)
	if requiredList, hasReq := raw["required"].([]interface{}); hasReq {
		for _, r := range requiredList {
			if rStr, ok := r.(string); ok {
				reqMap[rStr] = true
			}
		}
	}

	fields := make(map[string]FieldDefinition)
	for propName, propVal := range props {
		propMap, _ := propVal.(map[string]interface{})
		pType, _ := propMap["type"].(string)

		fields[propName] = FieldDefinition{
			Name:             propName,
			NormalizedType:   NormalizeType(pType),
			RawType:          pType,
			Optional:         !reqMap[propName],
			SerializationTag: propName,
		}
	}

	return []ContractSchema{
		{
			Name:       title,
			Type:       ContractJSONSchema,
			SourceFile: filename,
			Fields:     fields,
		},
	}, nil
}

// ParseOpenAPI extracts component schemas from OpenAPI 3.x specifications (JSON/YAML).
func ParseOpenAPI(content, filename string) ([]ContractSchema, error) {
	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSchemaContent, err)
	}

	components, ok := root["components"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	schemasObj, ok := components["schemas"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	var results []ContractSchema
	for schemaName, schemaVal := range schemasObj {
		sMap, _ := schemaVal.(map[string]interface{})
		props, _ := sMap["properties"].(map[string]interface{})

		reqMap := make(map[string]bool)
		if requiredList, hasReq := sMap["required"].([]interface{}); hasReq {
			for _, r := range requiredList {
				if rStr, ok := r.(string); ok {
					reqMap[rStr] = true
				}
			}
		}

		fields := make(map[string]FieldDefinition)
		for pName, pVal := range props {
			pMap, _ := pVal.(map[string]interface{})
			rawT, _ := pMap["type"].(string)
			if rawT == "integer" {
				if fmtStr, _ := pMap["format"].(string); fmtStr == "int64" {
					rawT = "int64"
				} else {
					rawT = "int32"
				}
			}

			fields[pName] = FieldDefinition{
				Name:             pName,
				NormalizedType:   NormalizeType(rawT),
				RawType:          rawT,
				Optional:         !reqMap[pName],
				SerializationTag: pName,
			}
		}

		results = append(results, ContractSchema{
			Name:       schemaName,
			Type:       ContractOpenAPI,
			SourceFile: filename,
			Fields:     fields,
		})
	}

	return results, nil
}

// ParseGoStructs parses Go source files and extracts struct fields and `json:"..."` tags.
func ParseGoStructs(content, filename string) ([]ContractSchema, error) {
	lines := strings.Split(content, "\n")
	var schemas []ContractSchema
	var currentSchema *ContractSchema

	structRegex := regexp.MustCompile(`^\s*type\s+([A-Za-z0-9_]+)\s+struct\s*\{`)
	fieldRegex := regexp.MustCompile(`^\s*([A-Za-z0-9_]+)\s+([*]?[A-Za-z0-9_\[\]]+)(\s+` + "`" + `([^` + "`" + `]+)` + "`" + `)?`)
	jsonTagRegex := regexp.MustCompile(`json:"([^",]+)`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		if matches := structRegex.FindStringSubmatch(line); len(matches) > 1 {
			if currentSchema != nil {
				schemas = append(schemas, *currentSchema)
			}
			currentSchema = &ContractSchema{
				Name:       matches[1],
				Language:   LangGo,
				SourceFile: filename,
				Fields:     make(map[string]FieldDefinition),
			}
			continue
		}

		if currentSchema != nil && trimmed == "}" {
			schemas = append(schemas, *currentSchema)
			currentSchema = nil
			continue
		}

		if currentSchema != nil {
			if fMatches := fieldRegex.FindStringSubmatch(line); len(fMatches) > 2 {
				name := fMatches[1]
				rawType := fMatches[2]
				tagContent := fMatches[4]

				tag := name
				if jMatches := jsonTagRegex.FindStringSubmatch(tagContent); len(jMatches) > 1 {
					tag = jMatches[1]
				}

				isOptional := strings.HasPrefix(rawType, "*") || strings.Contains(tagContent, "omitempty")

				currentSchema.Fields[tag] = FieldDefinition{
					Name:             name,
					NormalizedType:   NormalizeType(rawType),
					RawType:          rawType,
					Optional:         isOptional,
					SerializationTag: tag,
					LineNumber:       i + 1,
				}
			}
		}
	}

	if currentSchema != nil {
		schemas = append(schemas, *currentSchema)
	}

	return schemas, nil
}

// ParseRustSerdeStructs parses Rust structs and extracts serde tags and types.
func ParseRustSerdeStructs(content, filename string) ([]ContractSchema, error) {
	lines := strings.Split(content, "\n")
	var schemas []ContractSchema
	var currentSchema *ContractSchema

	structRegex := regexp.MustCompile(`^\s*(pub\s+)?struct\s+([A-Za-z0-9_]+)\s*\{`)
	fieldRegex := regexp.MustCompile(`^\s*(pub\s+)?([A-Za-z0-9_]+)\s*:\s*([^,;]+)`)
	serdeRenameRegex := regexp.MustCompile(`#\[serde\(rename\s*=\s*"([^"]+)"\)\]`)

	pendingRename := ""

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		if sMatches := serdeRenameRegex.FindStringSubmatch(line); len(sMatches) > 1 {
			pendingRename = sMatches[1]
			continue
		}

		if matches := structRegex.FindStringSubmatch(line); len(matches) > 2 {
			if currentSchema != nil {
				schemas = append(schemas, *currentSchema)
			}
			currentSchema = &ContractSchema{
				Name:       matches[2],
				Language:   LangRust,
				SourceFile: filename,
				Fields:     make(map[string]FieldDefinition),
			}
			continue
		}

		if currentSchema != nil && trimmed == "}" {
			schemas = append(schemas, *currentSchema)
			currentSchema = nil
			continue
		}

		if currentSchema != nil {
			if fMatches := fieldRegex.FindStringSubmatch(line); len(fMatches) > 3 {
				fieldName := fMatches[2]
				rawType := strings.TrimSpace(fMatches[3])

				tag := fieldName
				if pendingRename != "" {
					tag = pendingRename
					pendingRename = ""
				}

				isOptional := strings.HasPrefix(rawType, "Option<")

				currentSchema.Fields[tag] = FieldDefinition{
					Name:             fieldName,
					NormalizedType:   NormalizeType(rawType),
					RawType:          rawType,
					Optional:         isOptional,
					SerializationTag: tag,
					LineNumber:       i + 1,
				}
			}
		}
	}

	if currentSchema != nil {
		schemas = append(schemas, *currentSchema)
	}

	return schemas, nil
}

// ParseTypeScriptInterfaces extracts interface fields from TypeScript files.
func ParseTypeScriptInterfaces(content, filename string) ([]ContractSchema, error) {
	lines := strings.Split(content, "\n")
	var schemas []ContractSchema
	var currentSchema *ContractSchema

	interfaceRegex := regexp.MustCompile(`^\s*(export\s+)?interface\s+([A-Za-z0-9_]+)\s*\{`)
	fieldRegex := regexp.MustCompile(`^\s*([A-Za-z0-9_]+)(\?)?:\s*([^;]+);?`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		if matches := interfaceRegex.FindStringSubmatch(line); len(matches) > 2 {
			if currentSchema != nil {
				schemas = append(schemas, *currentSchema)
			}
			currentSchema = &ContractSchema{
				Name:       matches[2],
				Language:   LangTypeScript,
				SourceFile: filename,
				Fields:     make(map[string]FieldDefinition),
			}
			continue
		}

		if currentSchema != nil && trimmed == "}" {
			schemas = append(schemas, *currentSchema)
			currentSchema = nil
			continue
		}

		if currentSchema != nil {
			if fMatches := fieldRegex.FindStringSubmatch(line); len(fMatches) > 3 {
				name := fMatches[1]
				isOptional := fMatches[2] == "?"
				rawType := strings.TrimSpace(fMatches[3])

				currentSchema.Fields[name] = FieldDefinition{
					Name:             name,
					NormalizedType:   NormalizeType(rawType),
					RawType:          rawType,
					Optional:         isOptional,
					SerializationTag: name,
					LineNumber:       i + 1,
				}
			}
		}
	}

	if currentSchema != nil {
		schemas = append(schemas, *currentSchema)
	}

	return schemas, nil
}

// ParsePythonPydanticModels extracts Pydantic class models from Python files.
func ParsePythonPydanticModels(content, filename string) ([]ContractSchema, error) {
	lines := strings.Split(content, "\n")
	var schemas []ContractSchema
	var currentSchema *ContractSchema

	classRegex := regexp.MustCompile(`^\s*class\s+([A-Za-z0-9_]+)\((?:BaseModel|Schema)\):\s*`)
	fieldRegex := regexp.MustCompile(`^\s*([A-Za-z0-9_]+)\s*:\s*([^=]+)(?:=\s*Field\([^)]*alias="([^"]+)"[^)]*\))?`)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if matches := classRegex.FindStringSubmatch(line); len(matches) > 1 {
			if currentSchema != nil {
				schemas = append(schemas, *currentSchema)
			}
			currentSchema = &ContractSchema{
				Name:       matches[1],
				Language:   LangPython,
				SourceFile: filename,
				Fields:     make(map[string]FieldDefinition),
			}
			continue
		}

		if currentSchema != nil && len(line) > 0 && line[0] != ' ' && line[0] != '\t' && trimmed != "" {
			schemas = append(schemas, *currentSchema)
			currentSchema = nil
		}

		if currentSchema != nil {
			if fMatches := fieldRegex.FindStringSubmatch(line); len(fMatches) > 2 {
				name := fMatches[1]
				rawType := strings.TrimSpace(fMatches[2])
				alias := ""
				if len(fMatches) > 3 {
					alias = fMatches[3]
				}

				tag := name
				if alias != "" {
					tag = alias
				}

				isOptional := strings.HasPrefix(rawType, "Optional[")

				currentSchema.Fields[tag] = FieldDefinition{
					Name:             name,
					NormalizedType:   NormalizeType(rawType),
					RawType:          rawType,
					Optional:         isOptional,
					SerializationTag: tag,
					LineNumber:       i + 1,
				}
			}
		}
	}

	if currentSchema != nil {
		schemas = append(schemas, *currentSchema)
	}

	return schemas, nil
}
