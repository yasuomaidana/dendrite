package export

import (
	"fmt"
	"strings"
)

func pyType(f fieldDef) string {
	switch f.kind {
	case kindString:
		return "str"
	case kindFlag:
		return "bool"
	case kindList:
		elem := f.elem
		if elem == "string" {
			elem = "str"
		}
		return "tuple[" + elem + ", ...]"
	}
	return "int"
}

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

func renderPython(m Model, t value) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\"\"\"Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\"\"\"\n\nfrom __future__ import annotations\n\nfrom dataclasses import dataclass\n\n\n", commentSafe(m.Target), commentSafe(m.ID))
	for _, td := range schema {
		fmt.Fprintf(&sb, "@dataclass(frozen=True)\nclass %s:\n", td.name)
		for _, f := range td.fields {
			fmt.Fprintf(&sb, "    %s: %s\n", f.name, pyType(f))
		}
		sb.WriteString("\n\n")
	}
	sb.WriteString("IMAGE = " + pySyntax.literal(t, "") + "\n")
	return strings.ReplaceAll(sb.String(), "\t", "    ")
}
