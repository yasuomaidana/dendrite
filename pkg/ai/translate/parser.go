package translate

import (
	"bufio"
	"strings"
)

// ParsePythonToISR extracts an Intermediate Semantic Representation from Python source code.
func ParsePythonToISR(source string) (*ISRModule, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return nil, ErrEmptySource
	}

	module := &ISRModule{
		SourceLanguage: "python",
		ModuleName:     "main",
		Functions:      make([]ISRFunction, 0),
	}

	scanner := bufio.NewScanner(strings.NewReader(source))
	var currentFunc *ISRFunction

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Function definition: def func(a: int, b: str) -> bool:
		if strings.HasPrefix(line, "def ") || strings.HasPrefix(line, "async def ") {
			if currentFunc != nil {
				module.Functions = append(module.Functions, *currentFunc)
			}

			isAsync := strings.HasPrefix(line, "async def ")
			header := strings.TrimPrefix(line, "async def ")
			header = strings.TrimPrefix(header, "def ")

			funcName := header
			if idx := strings.Index(header, "("); idx != -1 {
				funcName = header[:idx]
			}

			params := parsePythonParams(header)
			retType := parsePythonReturnType(header)

			concurrency := ConcurrencyNone
			if isAsync {
				concurrency = ConcurrencyAsync
			}

			currentFunc = &ISRFunction{
				Name:           funcName,
				Parameters:     params,
				ReturnType:     retType,
				Concurrency:    concurrency,
				BodyStatements: make([]string, 0),
			}
			continue
		}

		if currentFunc != nil {
			// Check for exception raises
			if strings.HasPrefix(line, "raise ") {
				currentFunc.IsFallible = true
				parts := strings.Fields(line)
				if len(parts) > 1 {
					errName := parts[1]
					if pIdx := strings.Index(errName, "("); pIdx != -1 {
						errName = errName[:pIdx]
					}
					currentFunc.ErrorType = errName
				}
			}

			// Capture docstrings
			if strings.HasPrefix(line, "\"\"\"") || strings.HasPrefix(line, "'''") {
				currentFunc.Docstring = strings.Trim(line, "\"' ")
			}

			if line != "" && !strings.HasPrefix(line, "#") {
				currentFunc.BodyStatements = append(currentFunc.BodyStatements, line)
			}
		}
	}

	if currentFunc != nil {
		module.Functions = append(module.Functions, *currentFunc)
	}

	return module, nil
}

// ParseTypeScriptToISR extracts an Intermediate Semantic Representation from TypeScript source code.
func ParseTypeScriptToISR(source string) (*ISRModule, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return nil, ErrEmptySource
	}

	module := &ISRModule{
		SourceLanguage: "typescript",
		ModuleName:     "index",
		Functions:      make([]ISRFunction, 0),
	}

	scanner := bufio.NewScanner(strings.NewReader(source))
	var currentFunc *ISRFunction

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Function definition: export async function doSomething(param: string): Promise<number> {
		if strings.HasPrefix(line, "function ") || strings.HasPrefix(line, "export function ") ||
			strings.HasPrefix(line, "async function ") || strings.HasPrefix(line, "export async function ") {
			if currentFunc != nil {
				module.Functions = append(module.Functions, *currentFunc)
			}

			isAsync := strings.Contains(line, "async function")
			header := line
			for _, prefix := range []string{"export ", "async ", "function "} {
				header = strings.TrimPrefix(header, prefix)
			}

			funcName := header
			if idx := strings.Index(header, "("); idx != -1 {
				funcName = header[:idx]
			}

			params := parseTSParams(header)
			retType := parseTSReturnType(header)

			concurrency := ConcurrencyNone
			if isAsync {
				concurrency = ConcurrencyAsync
			}

			currentFunc = &ISRFunction{
				Name:           funcName,
				Parameters:     params,
				ReturnType:     retType,
				Concurrency:    concurrency,
				BodyStatements: make([]string, 0),
			}
			continue
		}

		if currentFunc != nil {
			if strings.HasPrefix(line, "throw ") {
				currentFunc.IsFallible = true
				currentFunc.ErrorType = "Error"
			}
			if line != "" && !strings.HasPrefix(line, "//") {
				currentFunc.BodyStatements = append(currentFunc.BodyStatements, line)
			}
		}
	}

	if currentFunc != nil {
		module.Functions = append(module.Functions, *currentFunc)
	}

	return module, nil
}

func parsePythonParams(header string) []ISRParameter {
	start := strings.Index(header, "(")
	end := strings.Index(header, ")")
	if start == -1 || end == -1 || start >= end {
		return nil
	}

	raw := header[start+1 : end]
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	params := make([]ISRParameter, 0, len(parts))

	for _, p := range parts {
		pTrimmed := strings.TrimSpace(p)
		if pTrimmed == "" || pTrimmed == "self" {
			continue
		}
		nameType := strings.Split(pTrimmed, ":")
		pName := strings.TrimSpace(nameType[0])
		pType := ISRType{Kind: TypeString, Name: "string"}
		if len(nameType) > 1 {
			typeStr := strings.TrimSpace(nameType[1])
			pType = normalizeType(typeStr)
		}
		params = append(params, ISRParameter{Name: pName, Type: pType})
	}
	return params
}

func parsePythonReturnType(header string) ISRType {
	if idx := strings.Index(header, "->"); idx != -1 {
		ret := strings.TrimSpace(header[idx+2:])
		ret = strings.TrimSuffix(ret, ":")
		return normalizeType(ret)
	}
	return ISRType{Kind: TypeVoid, Name: "void"}
}

func parseTSParams(header string) []ISRParameter {
	start := strings.Index(header, "(")
	end := strings.Index(header, ")")
	if start == -1 || end == -1 || start >= end {
		return nil
	}

	raw := header[start+1 : end]
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	params := make([]ISRParameter, 0, len(parts))

	for _, p := range parts {
		pTrimmed := strings.TrimSpace(p)
		if pTrimmed == "" {
			continue
		}
		nameType := strings.Split(pTrimmed, ":")
		pName := strings.TrimSpace(nameType[0])
		pType := ISRType{Kind: TypeString, Name: "string"}
		if len(nameType) > 1 {
			typeStr := strings.TrimSpace(nameType[1])
			pType = normalizeType(typeStr)
		}
		params = append(params, ISRParameter{Name: pName, Type: pType})
	}
	return params
}

func parseTSReturnType(header string) ISRType {
	endParen := strings.Index(header, ")")
	if endParen != -1 && endParen+1 < len(header) {
		after := strings.TrimSpace(header[endParen+1:])
		if strings.HasPrefix(after, ":") {
			ret := strings.TrimSpace(strings.TrimPrefix(after, ":"))
			ret = strings.TrimSuffix(ret, "{")
			ret = strings.TrimSpace(ret)
			// Unwrap Promise<T>
			if strings.HasPrefix(ret, "Promise<") && strings.HasSuffix(ret, ">") {
				inner := strings.TrimSuffix(strings.TrimPrefix(ret, "Promise<"), ">")
				return normalizeType(inner)
			}
			return normalizeType(ret)
		}
	}
	return ISRType{Kind: TypeVoid, Name: "void"}
}

func normalizeType(raw string) ISRType {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	switch trimmed {
	case "int", "integer", "number":
		return ISRType{Kind: TypeInt, Name: "int"}
	case "float", "double":
		return ISRType{Kind: TypeFloat, Name: "float"}
	case "str", "string":
		return ISRType{Kind: TypeString, Name: "string"}
	case "bool", "boolean":
		return ISRType{Kind: TypeBool, Name: "bool"}
	default:
		return ISRType{Kind: TypeCustom, Name: raw}
	}
}
