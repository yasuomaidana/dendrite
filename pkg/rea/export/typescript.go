package export

import (
	"fmt"
	"strings"
)

func tsType(f fieldDef) string {
	switch f.kind {
	case kindString:
		return "string"
	case kindFlag:
		return "boolean"
	case kindAddress:
		return "bigint"
	case kindList:
		return "readonly " + f.elem + "[]"
	}
	return "number"
}

var tsSyntax = syntax{
	str:  jsonString,
	num:  func(n uint64) string { return fmt.Sprint(n) },
	addr: func(n uint64) string { return fmt.Sprintf("0x%xn", n) },
	flag: func(b bool) string { return fmt.Sprint(b) },
	list: func(_ string, items []string, indent string) string {
		if len(items) == 0 {
			return "[]"
		}
		return "[\n" + indent + "\t" + strings.Join(items, ",\n"+indent+"\t") + ",\n" + indent + "]"
	},
	rec: func(_ string, names, vals []string, indent string) string {
		var sb strings.Builder
		sb.WriteString("{\n")
		for i := range names {
			fmt.Fprintf(&sb, "%s\t%s: %s,\n", indent, names[i], vals[i])
		}
		return sb.String() + indent + "}"
	},
}

func renderTypeScript(m Model, t value) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\n\n", commentSafe(m.Target), commentSafe(m.ID))
	for _, td := range schema {
		fmt.Fprintf(&sb, "export interface %s {\n", td.name)
		for _, f := range td.fields {
			fmt.Fprintf(&sb, "  readonly %s: %s;\n", f.name, tsType(f))
		}
		sb.WriteString("}\n\n")
	}
	sb.WriteString("export const IMAGE: Image = " + tsSyntax.literal(t, "") + ";\n")
	return strings.ReplaceAll(sb.String(), "\t", "  ")
}
