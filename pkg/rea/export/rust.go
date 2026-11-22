package export

import (
	"fmt"
	"strings"
)

// rustString quotes s as a Rust string literal, keeping printable ASCII and writing every other character as \u{..}.
func rustString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			sb.WriteByte('\\')
			sb.WriteRune(r)
		case r >= 0x20 && r < 0x7f:
			sb.WriteRune(r)
		default:
			fmt.Fprintf(&sb, "\\u{%x}", r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

var rustSyntax = syntax{
	str:  rustString,
	num:  func(n uint64) string { return fmt.Sprint(n) },
	addr: func(n uint64) string { return fmt.Sprintf("0x%x", n) },
	flag: func(b bool) string { return fmt.Sprint(b) },
	list: func(_ string, items []string, indent string) string {
		if len(items) == 0 {
			return "&[]"
		}
		return "&[\n" + indent + "\t" + strings.Join(items, ",\n"+indent+"\t") + ",\n" + indent + "]"
	},
	rec: func(typ string, names, vals []string, indent string) string {
		var sb strings.Builder
		sb.WriteString(typ + " {\n")
		for i := range names {
			fmt.Fprintf(&sb, "%s\t%s: %s,\n", indent, names[i], vals[i])
		}
		return sb.String() + indent + "}"
	},
}

var rustLang = language{
	ext: "rs", sx: rustSyntax, tab: "    ",
	header:   "//! Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\n\n",
	prologue: "#![allow(dead_code)]\n\n",
	typeOpen: "#[derive(Debug, Clone, Copy, PartialEq)]\npub struct %s {\n", typeField: "\tpub %s: %s,\n", close: "}\n\n",
	tStr: "&'static str", tNum: "u64", tAddr: "u64", tFlag: "bool",
	tList: func(elem string) string { return "&'static [" + elem + "]" },
	tail:  "pub const IMAGE: Image = %s;\n",
}
