package export

import (
	"fmt"
	"strings"
)

var pySyntax = syntax{
	str:  jsonString,
	num:  func(n uint64) string { return fmt.Sprint(n) },
	addr: func(n uint64) string { return fmt.Sprintf("0x%x", n) },
	flag: func(b bool) string {
		if b {
			return "True"
		}
		return "False"
	},
	list: func(_ string, items []string, indent string) string {
		if len(items) == 0 {
			return "()"
		}
		return "(\n" + indent + "\t" + strings.Join(items, ",\n"+indent+"\t") + ",\n" + indent + ")"
	},
	rec: func(typ string, names, vals []string, indent string) string {
		var sb strings.Builder
		sb.WriteString(typ + "(\n")
		for i := range names {
			fmt.Fprintf(&sb, "%s\t%s=%s,\n", indent, names[i], vals[i])
		}
		return sb.String() + indent + ")"
	},
}

var pyLang = language{
	ext: "py", sx: pySyntax, tab: "    ",
	header:   "\"\"\"Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\"\"\"\n\nfrom __future__ import annotations\n\nfrom dataclasses import dataclass\n\n\n",
	typeOpen: "@dataclass(frozen=True)\nclass %s:\n", typeField: "\t%s: %s\n", close: "\n\n",
	tStr: "str", tNum: "int", tAddr: "int", tFlag: "bool",
	tList: func(elem string) string { return "tuple[" + elem + ", ...]" },
	tail:  "IMAGE = %s\n",
}
