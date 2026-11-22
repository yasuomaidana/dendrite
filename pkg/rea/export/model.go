// Package export writes a finished REA analysis as a plain data module in Rust, Go, Python or TypeScript: the
// sections, libraries and symbols, exports, files, notes and the functions an engine found, for a target of any
// format. The module holds data only (no code to run, no dependencies, no unsafe code), so it can be checked in,
// compiled and used as the ground truth of a cleanroom recreation.
package export

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/rea/engine"
)

// Model is what an export writes: an analysis of any format, language-neutral.
type Model struct {
	ID, Target string
	Image      *coreclient.Image
	Functions  []engine.Function
}

// NewModel builds the model of analysis id of the target at path from what its inspection and engine found.
func NewModel(id, target string, img *coreclient.Image, fns []engine.Function) Model {
	return Model{ID: id, Target: filepath.Base(target), Image: img, Functions: fns}
}

// parseAddr reads a hex or decimal address as the inspectors write it; an empty one (a target with no entry point,
// such as a package) is zero.
func parseAddr(s string) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("bad address %q: %w", s, err)
	}
	return n, nil
}

// commentSafe makes text fit on one line of a comment: control characters and the closers of block comments and
// docstrings become spaces, so a hostile file name cannot end the comment and start code.
func commentSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	for _, closer := range []string{"*/", `"""`, "'''"} {
		s = strings.ReplaceAll(s, closer, "  ")
	}
	return s
}

// value is a node of the tree an export is built as: a string, a number, an address, a flag, a list or a record.
type value struct {
	kind   valueKind
	s      string
	n      uint64
	b      bool
	typ    string // record type, or the element type of a list
	fields []field
	items  []value
}

type valueKind int

const (
	kindString valueKind = iota
	kindNumber
	kindAddress
	kindFlag
	kindList
	kindRecord
)

type field struct {
	name string
	v    value
}

func str(s string) value                 { return value{kind: kindString, s: s} }
func num(n uint64) value                 { return value{kind: kindNumber, n: n} }
func addr(n uint64) value                { return value{kind: kindAddress, n: n} }
func flag(b bool) value                  { return value{kind: kindFlag, b: b} }
func list(elem string, v ...value) value { return value{kind: kindList, typ: elem, items: v} }
func rec(typ string, f ...field) value   { return value{kind: kindRecord, typ: typ, fields: f} }
func f(name string, v value) field       { return field{name, v} }

func strings_(items []string) value {
	out := make([]value, len(items))
	for i, it := range items {
		out[i] = str(it)
	}
	return list("string", out...)
}

// tree is the model as a value: an Image record holding lists of Section, Library, File, Function and Note records.
func (m Model) tree() (value, error) {
	img := m.Image
	entry, err := parseAddr(img.Entry)
	if err != nil {
		return value{}, err
	}
	base, err := parseAddr(img.Base)
	if err != nil {
		return value{}, err
	}
	sections, err := m.sections()
	if err != nil {
		return value{}, err
	}
	fns, err := m.functions()
	if err != nil {
		return value{}, err
	}
	return rec("Image",
		f("kind", str(img.Kind)), f("format", str(img.Format)), f("arch", str(img.Arch)), f("bits", num(uint64(img.Bits))),
		f("endian", str(img.Endian)), f("entry", addr(entry)), f("base", addr(base)),
		f("sections", sections), f("libraries", m.libraries()), f("exports", strings_(img.Exports)),
		f("files", m.files()), f("functions", fns), f("notes", m.notes()), f("truncated", flag(img.Truncated))), nil
}

func (m Model) sections() (value, error) {
	items := make([]value, 0, len(m.Image.Sections))
	for _, s := range m.Image.Sections {
		a, err := parseAddr(s.Address)
		if err != nil {
			return value{}, err
		}
		items = append(items, rec("Section", f("name", str(s.Name)), f("address", addr(a)), f("size", num(s.Size)), f("raw_size", num(s.RawSize)), f("flags", strings_(s.Flags))))
	}
	return list("Section", items...), nil
}

func (m Model) libraries() value {
	items := make([]value, 0, len(m.Image.Libraries))
	for _, l := range m.Image.Libraries {
		items = append(items, rec("Library", f("name", str(l.Name)), f("symbols", strings_(l.Symbols))))
	}
	return list("Library", items...)
}

func (m Model) files() value {
	items := make([]value, 0, len(m.Image.Files))
	for _, fl := range m.Image.Files {
		items = append(items, rec("File", f("path", str(fl.Path)), f("size", num(fl.Size)), f("kind", str(fl.Kind))))
	}
	return list("File", items...)
}

func (m Model) functions() (value, error) {
	items := make([]value, 0, len(m.Functions))
	for _, fn := range m.Functions {
		a, err := parseAddr(fn.Address)
		if err != nil {
			return value{}, err
		}
		items = append(items, rec("Function", f("name", str(fn.Name)), f("address", addr(a)), f("size", num(fn.Size)), f("cyclomatic", num(uint64(max(fn.Cyclomatic, 0)))), f("calls", strings_(fn.Calls))))
	}
	return list("Function", items...), nil
}

// notes are the format specifics (subsystem, interpreter, code signature, ...) in a stable order.
func (m Model) notes() value {
	keys := make([]string, 0, len(m.Image.Notes))
	for k := range m.Image.Notes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	items := make([]value, 0, len(keys))
	for _, k := range keys {
		items = append(items, rec("Note", f("key", str(k)), f("value", str(m.Image.Notes[k]))))
	}
	return list("Note", items...)
}
