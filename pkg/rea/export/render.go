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

// literal writes v in the language; indent is the indentation of the line the value starts on. Lists and records go one
// item per line; "$" in listOpen, recOpen and field stands for the element type, the record type and the field name.
func (l language) literal(v value, indent string) string {
	inner := indent + "\t"
	switch v.kind {
	case kindString:
		return l.str(v.s)
	case kindNumber:
		return fmt.Sprint(v.n)
	case kindAddress:
		return fmt.Sprintf("0x%x%s", v.n, l.suffix)
	case kindFlag:
		if v.b {
			return l.yes
		}
		return l.no
	case kindList:
		open := strings.ReplaceAll(l.listOpen, "$", v.typ)
		if len(v.items) == 0 {
			return open + l.listClose
		}
		items := make([]string, len(v.items))
		for i, it := range v.items {
			items[i] = l.literal(it, inner)
		}
		return open + "\n" + inner + strings.Join(items, ",\n"+inner) + ",\n" + indent + l.listClose
	}
	var sb strings.Builder
	sb.WriteString(strings.ReplaceAll(l.recOpen, "$", v.typ) + "\n")
	for _, fl := range v.fields {
		sb.WriteString(inner + strings.ReplaceAll(l.field, "$", l.name(fl.name)) + l.literal(fl.v, inner) + ",\n")
	}
	return sb.String() + indent + l.recClose
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
	ext, tab                   string // file extension; what replaces the tabs the layout is built with
	str                        func(string) string
	suffix, yes, no            string // after an address; the words for true and false
	listOpen, listClose        string
	recOpen, field, recClose   string
	header, prologue           string // header's formats: target name, analysis id
	typeOpen, typeField, close string // formats: type name; field name and type
	fieldName                  func(string) string
	tStr, tNum, tAddr, tFlag   string
	tList                      string // "$" is the element type
	tail                       string // format: the literal of the image
}

// name is a field name as the language writes it.
func (l language) name(n string) string {
	if l.fieldName != nil {
		return l.fieldName(n)
	}
	return n
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
		return strings.ReplaceAll(l.tList, "$", elem)
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
			fmt.Fprintf(&sb, l.typeField, l.name(f.name), l.typeOf(f))
		}
		sb.WriteString(l.close)
	}
	fmt.Fprintf(&sb, l.tail, l.literal(t, ""))
	return strings.ReplaceAll(sb.String(), "\t", l.tab)
}
