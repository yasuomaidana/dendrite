package export

import (
	"fmt"
	"strconv"
	"strings"
)

func goType(f fieldDef) string {
	switch f.kind {
	case kindString:
		return "string"
	case kindFlag:
		return "bool"
	case kindList:
		return "[]" + f.elem
	}
	return "uint64"
}

var goSyntax = syntax{
	str:  strconv.Quote,
	num:  func(n uint64) string { return fmt.Sprint(n) },
	addr: func(n uint64) string { return fmt.Sprintf("0x%x", n) },
	flag: func(b bool) string { return fmt.Sprint(b) },
	list: func(elem string, items []string, indent string) string {
		if len(items) == 0 {
			return "[]" + elem + "{}"
		}
		return "[]" + elem + "{\n" + indent + "\t" + strings.Join(items, ",\n"+indent+"\t") + ",\n" + indent + "}"
	},
	rec: func(typ string, names, vals []string, indent string) string {
		var sb strings.Builder
		sb.WriteString(typ + "{\n")
		for i := range names {
			fmt.Fprintf(&sb, "%s\t%s: %s,\n", indent, camel(names[i]), vals[i])
		}
		return sb.String() + indent + "}"
	},
}

func renderGo(m Model, t value) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "// Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\npackage analysis\n\n", commentSafe(m.Target), commentSafe(m.ID))
	for _, td := range schema {
		fmt.Fprintf(&sb, "type %s struct {\n", td.name)
		for _, f := range td.fields {
			fmt.Fprintf(&sb, "\t%s %s\n", camel(f.name), goType(f))
		}
		sb.WriteString("}\n\n")
	}
	sb.WriteString("var Analysis = " + goSyntax.literal(t, "") + "\n")
	return sb.String()
}
