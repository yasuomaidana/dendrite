package export

import (
	"fmt"
	"strconv"
	"strings"
)

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

var goLang = language{
	ext: "go", sx: goSyntax, tab: "\t",
	header:   "// Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\npackage analysis\n\n",
	typeOpen: "type %s struct {\n", typeField: "\t%s %s\n", close: "}\n\n", fieldName: camel,
	tStr: "string", tNum: "uint64", tAddr: "uint64", tFlag: "bool",
	tList: func(elem string) string { return "[]" + elem },
	tail:  "var Analysis = %s\n",
}
