package export

import (
	"fmt"
	"strings"
)

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

var tsLang = language{
	ext: "ts", sx: tsSyntax, tab: "  ",
	header:   "// Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\n\n",
	typeOpen: "export interface %s {\n", typeField: "\treadonly %s: %s;\n", close: "}\n\n",
	tStr: "string", tNum: "number", tAddr: "bigint", tFlag: "boolean",
	tList: func(elem string) string { return "readonly " + elem + "[]" },
	tail:  "export const IMAGE: Image = %s;\n",
}
