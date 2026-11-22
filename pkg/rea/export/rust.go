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

func rustType(f fieldDef) string {
	switch f.kind {
	case kindString:
		return "&'static str"
	case kindFlag:
		return "bool"
	case kindList:
		elem := f.elem
		if elem == "string" {
			elem = "&'static str"
		}
		return "&'static [" + elem + "]"
	}
	return "u64"
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

func renderRust(m Model, t value) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "//! Structure of %s (analysis %s), written by `dendrite retro export`. Data only: no code, no dependencies.\n\n", commentSafe(m.Target), commentSafe(m.ID))
	sb.WriteString("#![allow(dead_code)]\n\n")
	for _, td := range schema {
		fmt.Fprintf(&sb, "#[derive(Debug, Clone, Copy, PartialEq)]\npub struct %s {\n", td.name)
		for _, f := range td.fields {
			fmt.Fprintf(&sb, "    pub %s: %s,\n", f.name, rustType(f))
		}
		sb.WriteString("}\n\n")
	}
	sb.WriteString("pub const IMAGE: Image = " + rustSyntax.literal(t, "") + ";\n")
	return strings.ReplaceAll(sb.String(), "\t", "    ")
}
