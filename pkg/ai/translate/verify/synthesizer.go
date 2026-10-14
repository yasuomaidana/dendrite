package verify

import (
	"fmt"
	"strings"

	"Dendrite/pkg/ai/translate"
)

// SynthesizeGoFromISR transforms a language-neutral ISRModule into idiomatic, compilable Go code.
func SynthesizeGoFromISR(mod *translate.ISRModule, targetPkg string) (string, error) {
	if mod == nil {
		return "", fmt.Errorf("ISR module cannot be nil")
	}

	if targetPkg == "" {
		targetPkg = "translated"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("package %s\n\n", targetPkg))

	needsContext := false
	needsErrors := false

	for _, fn := range mod.Functions {
		if fn.Concurrency == translate.ConcurrencyAsync {
			needsContext = true
		}
		if fn.IsFallible {
			needsErrors = true
		}
	}

	if needsContext || needsErrors {
		sb.WriteString("import (\n")
		if needsContext {
			sb.WriteString("\t\"context\"\n")
		}
		if needsErrors {
			sb.WriteString("\t\"fmt\"\n")
		}
		sb.WriteString(")\n\n")
	}

	for _, fn := range mod.Functions {
		if fn.Docstring != "" {
			sb.WriteString(fmt.Sprintf("// %s %s\n", fn.Name, fn.Docstring))
		} else {
			sb.WriteString(fmt.Sprintf("// %s was synthesized from %s source.\n", fn.Name, mod.SourceLanguage))
		}

		paramParts := make([]string, 0, len(fn.Parameters)+1)
		if fn.Concurrency == translate.ConcurrencyAsync {
			paramParts = append(paramParts, "ctx context.Context")
		}

		for _, p := range fn.Parameters {
			goType := mapISRTypeToGo(p.Type)
			paramParts = append(paramParts, fmt.Sprintf("%s %s", p.Name, goType))
		}

		var retSig string
		goRet := mapISRTypeToGo(fn.ReturnType)

		if fn.IsFallible {
			if goRet == "" {
				retSig = "error"
			} else {
				retSig = fmt.Sprintf("(%s, error)", goRet)
			}
		} else {
			retSig = goRet
		}

		if retSig != "" {
			sb.WriteString(fmt.Sprintf("func %s(%s) %s {\n", fn.Name, strings.Join(paramParts, ", "), retSig))
		} else {
			sb.WriteString(fmt.Sprintf("func %s(%s) {\n", fn.Name, strings.Join(paramParts, ", ")))
		}

		// Synthesize body statements
		if fn.IsFallible {
			sb.WriteString("\tif false {\n\t\treturn ")
			if goRet != "" {
				sb.WriteString(fmt.Sprintf("%s, ", defaultZeroValue(fn.ReturnType)))
			}
			sb.WriteString(fmt.Sprintf("fmt.Errorf(%q)\n\t}\n", fn.ErrorType))
		}

		if goRet != "" {
			sb.WriteString(fmt.Sprintf("\treturn %s", defaultZeroValue(fn.ReturnType)))
			if fn.IsFallible {
				sb.WriteString(", nil")
			}
			sb.WriteString("\n")
		} else if fn.IsFallible {
			sb.WriteString("\treturn nil\n")
		}

		sb.WriteString("}\n\n")
	}

	return sb.String(), nil
}

func mapISRTypeToGo(t translate.ISRType) string {
	switch t.Kind {
	case translate.TypeInt:
		return "int"
	case translate.TypeFloat:
		return "float64"
	case translate.TypeString:
		return "string"
	case translate.TypeBool:
		return "bool"
	case translate.TypeVoid:
		return ""
	default:
		if t.Name != "" && t.Name != "void" {
			return t.Name
		}
		return "interface{}"
	}
}

func defaultZeroValue(t translate.ISRType) string {
	switch t.Kind {
	case translate.TypeInt:
		return "0"
	case translate.TypeFloat:
		return "0.0"
	case translate.TypeString:
		return `""`
	case translate.TypeBool:
		return "false"
	default:
		return "nil"
	}
}
