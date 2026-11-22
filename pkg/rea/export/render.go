package export

import (
	"encoding/json"
	"fmt"
	"strings"
)

// fieldDef and typeDef describe the records of a module, in the order they are declared.
type fieldDef struct {
	name string
	kind valueKind
	elem string // for a list, the type of its items: "string" or a record name
}

type typeDef struct {
	name   string
	fields []fieldDef
}

func s_(n string) fieldDef       { return fieldDef{name: n, kind: kindString} }
func n_(n string) fieldDef       { return fieldDef{name: n, kind: kindNumber} }
func a_(n string) fieldDef       { return fieldDef{name: n, kind: kindAddress} }
func l_(n, elem string) fieldDef { return fieldDef{name: n, kind: kindList, elem: elem} }
func b_(n string) fieldDef       { return fieldDef{name: n, kind: kindFlag} }

// schema is every record of the module, a type before the types that use it.
var schema = []typeDef{
	{"Section", []fieldDef{s_("name"), a_("address"), n_("size"), n_("raw_size"), l_("flags", "string")}},
	{"Library", []fieldDef{s_("name"), l_("symbols", "string")}},
	{"File", []fieldDef{s_("path"), n_("size"), s_("kind")}},
	{"Function", []fieldDef{s_("name"), a_("address"), n_("size"), n_("cyclomatic"), l_("calls", "string")}},
	{"Note", []fieldDef{s_("key"), s_("value")}},
	{"Image", []fieldDef{s_("kind"), s_("format"), s_("arch"), n_("bits"), s_("endian"), a_("entry"), a_("base"),
		l_("sections", "Section"), l_("libraries", "Library"), l_("exports", "string"), l_("files", "File"),
		l_("functions", "Function"), l_("notes", "Note"), b_("truncated")}},
}

// syntax is what differs between languages when a value is written: the literal of each scalar, and how a list and a
// record are laid out from their already written parts.
type syntax struct {
	str  func(string) string
	num  func(uint64) string
	addr func(uint64) string
	flag func(bool) string
	list func(elem string, items []string, indent string) string
	rec  func(typ string, names, vals []string, indent string) string
}

// literal writes v with the syntax of a language; indent is the indentation of the line the value starts on.
func (sx syntax) literal(v value, indent string) string {
	inner := indent + "\t"
	switch v.kind {
	case kindString:
		return sx.str(v.s)
	case kindNumber:
		return sx.num(v.n)
	case kindAddress:
		return sx.addr(v.n)
	case kindFlag:
		return sx.flag(v.b)
	case kindList:
		items := make([]string, len(v.items))
		for i, it := range v.items {
			items[i] = sx.literal(it, inner)
		}
		return sx.list(v.typ, items, indent)
	}
	names, vals := make([]string, len(v.fields)), make([]string, len(v.fields))
	for i, fl := range v.fields {
		names[i], vals[i] = fl.name, sx.literal(fl.v, inner)
	}
	return sx.rec(v.typ, names, vals, indent)
}

// jsonString quotes s as a JSON string, which is also a valid string literal in Python and TypeScript.
func jsonString(s string) string {
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string always encodes
	return strings.TrimSuffix(sb.String(), "\n")
}

// camel turns snake_case into CamelCase.
func camel(s string) string {
	var sb strings.Builder
	for _, part := range strings.Split(s, "_") {
		sb.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return sb.String()
}

// language is one target language: its file extension, how values are written and how the types are declared.
type language struct {
	ext                        string
	sx                         syntax
	tab                        string // replaces the tabs the layout is built with
	header                     string // format: target name, analysis id
	prologue                   string
	typeOpen, typeField, close string // formats: type name; field name and type
	fieldName                  func(string) string
	tStr, tNum, tAddr, tFlag   string
	tList                      func(elem string) string
	tail                       string // format: the literal of the image
}

// typeOf is the declared type of a field in the language.
func (l language) typeOf(f fieldDef) string {
	elem := f.elem
	if elem == "string" {
		elem = l.tStr
	}
	switch f.kind {
	case kindString:
		return l.tStr
	case kindFlag:
		return l.tFlag
	case kindAddress:
		return l.tAddr
	case kindList:
		return l.tList(elem)
	}
	return l.tNum
}

// module writes the whole file: header, type declarations and the image.
func (l language) module(m Model, t value) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, l.header, commentSafe(m.Target), commentSafe(m.ID))
	sb.WriteString(l.prologue)
	for _, td := range schema {
		fmt.Fprintf(&sb, l.typeOpen, td.name)
		for _, f := range td.fields {
			name := f.name
			if l.fieldName != nil {
				name = l.fieldName(name)
			}
			fmt.Fprintf(&sb, l.typeField, name, l.typeOf(f))
		}
		sb.WriteString(l.close)
	}
	fmt.Fprintf(&sb, l.tail, l.sx.literal(t, ""))
	return strings.ReplaceAll(sb.String(), "\t", l.tab)
}
