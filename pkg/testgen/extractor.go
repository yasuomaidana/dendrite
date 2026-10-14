package testgen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"unicode"
)

// ExtractSymbols parses a source file and extracts all exported functions, structs, and methods.
func ExtractSymbols(filePath string) ([]SourceSymbol, string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read source file: %w", err)
	}

	return ExtractSymbolsFromSource(string(data), filePath)
}

// ExtractSymbolsFromSource parses Go code and returns symbol descriptors and the package name.
func ExtractSymbolsFromSource(source, filename string) ([]SourceSymbol, string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, source, parser.ParseComments)
	if err != nil {
		return nil, "", fmt.Errorf("ast parse error: %w", err)
	}

	pkgName := node.Name.Name
	var symbols []SourceSymbol

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := parseFuncDecl(d)
			symbols = append(symbols, sym)

		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if _, isStruct := typeSpec.Type.(*ast.StructType); isStruct {
							symbols = append(symbols, SourceSymbol{
								Name:       typeSpec.Name.Name,
								Kind:       "struct",
								IsExported: isExported(typeSpec.Name.Name),
								Doc:        d.Doc.Text(),
							})
						}
					}
				}
			}
		}
	}

	return symbols, pkgName, nil
}

func parseFuncDecl(d *ast.FuncDecl) SourceSymbol {
	name := d.Name.Name
	kind := "func"
	receiver := ""

	if d.Recv != nil && len(d.Recv.List) > 0 {
		kind = "method"
		receiver = formatType(d.Recv.List[0].Type)
	}

	var params []string
	if d.Type.Params != nil {
		for _, p := range d.Type.Params.List {
			pType := formatType(p.Type)
			for range p.Names {
				params = append(params, pType)
			}
			if len(p.Names) == 0 {
				params = append(params, pType)
			}
		}
	}

	var returns []string
	if d.Type.Results != nil {
		for _, r := range d.Type.Results.List {
			rType := formatType(r.Type)
			returns = append(returns, rType)
		}
	}

	sig := fmt.Sprintf("%s(%s)", name, strings.Join(params, ", "))
	if len(returns) > 0 {
		sig = fmt.Sprintf("%s (%s)", sig, strings.Join(returns, ", "))
	}

	return SourceSymbol{
		Name:        name,
		Kind:        kind,
		Receiver:    receiver,
		Signature:   sig,
		ParamTypes:  params,
		ReturnTypes: returns,
		IsExported:  isExported(name),
		Doc:         d.Doc.Text(),
	}
}

func formatType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + formatType(t.X)
	case *ast.SelectorExpr:
		return formatType(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + formatType(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", formatType(t.Key), formatType(t.Value))
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return "interface{}"
	}
}

func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	r := []rune(name)[0]
	return unicode.IsUpper(r)
}
