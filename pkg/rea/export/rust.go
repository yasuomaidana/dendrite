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
	str: rustString, num: decimal, addr: hexAddr(""), flag: boolLit("true", "false"),
	list: listFn(fixed("&["), "]"),
	rec:  recFn(func(typ string) string { return typ + " {" }, func(n string) string { return n + ": " }, "}"),
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
