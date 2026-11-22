package analysis

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/fsutil"
)

// ErrNeedsOutput means no output directory was given: an export never guesses where to write.
var ErrNeedsOutput = errors.New("an output directory is required: pass --output")

// ErrExportUnsupported means the export knows no output for the format of the analysis yet (see DEN-1065).
var ErrExportUnsupported = errors.New("export only writes PE images for now")

// peOf is the PE structure of an image of format pe, the shape the Rust export renders.
func peOf(img *Image) *coreclient.PEInfo {
	pe := &coreclient.PEInfo{Machine: img.Arch, Subsystem: img.Notes["subsystem"], PE32Plus: img.Bits == 64, EntryPoint: img.Entry,
		ImageBase: img.Base, Exports: img.Exports, Truncated: img.Truncated}
	for _, sec := range img.Sections {
		pe.Sections = append(pe.Sections, coreclient.PESection{Name: sec.Name, VirtualAddress: sec.Address, VirtualSize: sec.Size, RawSize: sec.RawSize, Flags: sec.Flags})
	}
	for _, lib := range img.Libraries {
		pe.Imports = append(pe.Imports, coreclient.PEImport{DLL: lib.Name, Functions: lib.Symbols})
	}
	return pe
}

// ExportRust writes the structure of a finished analysis as a safe Rust module, <output>/<id>.rs, and returns its
// path. The module holds plain data (no unsafe code, no dependencies), so it compiles on its own.
func (s *Service) ExportRust(id, output string) (string, error) {
	if strings.TrimSpace(output) == "" {
		return "", ErrNeedsOutput
	}
	st, err := s.Store.Load(id)
	if err != nil {
		return "", err
	}
	if st.Stage != StageDone {
		return "", ErrNotDone
	}
	img, err := s.loadImage(id)
	if err != nil {
		return "", err
	}
	if img.Format != coreclient.FormatPE {
		return "", fmt.Errorf("%w: analysis %s is a %s target", ErrExportUnsupported, id, img.Format)
	}
	src, err := rustModule(st, peOf(img))
	if err != nil {
		return "", fmt.Errorf("analysis %s: %w", id, err)
	}
	path := filepath.Join(output, id+".rs") // id is a validated file name, so the file stays under output
	return path, fsutil.WriteFileAtomic(path, []byte(src))
}

// rustString quotes s as a Rust string literal, keeping ASCII text as it is and every other character as \u{..}.
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

// rustAddress turns a hex address string of the parser into a Rust u64 literal.
func rustAddress(addr string) (string, error) {
	n, err := strconv.ParseUint(addr, 0, 64)
	if err != nil {
		return "", fmt.Errorf("bad address %q: %w", addr, err)
	}
	return fmt.Sprintf("0x%x", n), nil
}

const rustTypes = `
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Section {
    pub name: &'static str,
    pub virtual_address: u64,
    pub virtual_size: u64,
    pub raw_size: u64,
    pub flags: &'static [&'static str],
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Import {
    pub dll: &'static str,
    pub functions: &'static [&'static str],
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Image {
    pub machine: &'static str,
    pub subsystem: &'static str,
    pub pe32_plus: bool,
    pub entry_point: u64,
    pub image_base: u64,
    pub sections: &'static [Section],
    pub imports: &'static [Import],
    pub exports: &'static [&'static str],
    /// The parser cut the import and export lists: they are not complete.
    pub truncated: bool,
}
`

// rustModule renders the analysis as the source of a Rust module.
func rustModule(st State, info *coreclient.PEInfo) (string, error) {
	entry, err := rustAddress(info.EntryPoint)
	if err != nil {
		return "", err
	}
	base, err := rustAddress(info.ImageBase)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "//! PE image structure of analysis %s, written by `dendrite retro export`. Plain data, no unsafe code.\n", st.ID)
	sb.WriteString(rustTypes)
	sb.WriteString("\npub const IMAGE: Image = Image {\n")
	fmt.Fprintf(&sb, "    machine: %s,\n    subsystem: %s,\n    pe32_plus: %t,\n    entry_point: %s,\n    image_base: %s,\n",
		rustString(info.Machine), rustString(info.Subsystem), info.PE32Plus, entry, base)
	sb.WriteString("    sections: &[\n")
	for _, sec := range info.Sections {
		va, err := rustAddress(sec.VirtualAddress)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, "        Section { name: %s, virtual_address: %s, virtual_size: %d, raw_size: %d, flags: &[%s] },\n",
			rustString(sec.Name), va, sec.VirtualSize, sec.RawSize, rustList(sec.Flags))
	}
	sb.WriteString("    ],\n    imports: &[\n")
	for _, imp := range info.Imports {
		fmt.Fprintf(&sb, "        Import { dll: %s, functions: &[%s] },\n", rustString(imp.DLL), rustList(imp.Functions))
	}
	fmt.Fprintf(&sb, "    ],\n    exports: &[%s],\n    truncated: %t,\n};\n", rustList(info.Exports), info.Truncated)
	return sb.String(), nil
}

// rustList renders strings as the comma separated items of a Rust slice.
func rustList(items []string) string {
	quoted := make([]string, len(items))
	for i, it := range items {
		quoted[i] = rustString(it)
	}
	return strings.Join(quoted, ", ")
}
