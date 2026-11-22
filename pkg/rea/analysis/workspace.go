package analysis

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/rea"
)

var unsafeName = regexp.MustCompile(`[^a-z0-9._-]+`)

// docName makes a document name from a library name: lower case, safe characters only, and unique among used.
func docName(name string, used map[string]int) string {
	base := strings.Trim(unsafeName.ReplaceAllString(strings.ToLower(filepath.Base(name)), "_"), "._-")
	if base == "" {
		base = "library"
	}
	n := used[base]
	used[base]++
	if n > 0 {
		base = fmt.Sprintf("%s_%d", base, n+1)
	}
	return base + ".md"
}

// libraryLabel names what an image of this format depends on.
func libraryLabel(format string) string {
	switch format {
	case coreclient.FormatPE:
		return "Imported DLLs"
	case coreclient.FormatELF:
		return "Shared libraries (DT_NEEDED)"
	case coreclient.FormatMachO, coreclient.FormatFatMachO:
		return "Dylibs"
	case coreclient.FormatASAR, coreclient.FormatZIP, coreclient.FormatJSBundle, coreclient.FormatSourceTree:
		return "Dependencies"
	}
	return "Libraries"
}

// overview is the first document: the facts of the target and where the rest is.
func overview(st State, img *Image) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Analysis %s: %s\n\n", st.ID, filepath.Base(st.Binary))
	fmt.Fprintf(&sb, "- Kind: %s\n- Format: %s\n", img.Kind, img.Format)
	for _, f := range [][2]string{{"Architecture", img.Arch}, {"Endian", img.Endian}, {"Entry point", img.Entry}, {"Base address", img.Base}} {
		if f[1] != "" {
			fmt.Fprintf(&sb, "- %s: %s\n", f[0], f[1])
		}
	}
	if img.Bits > 0 {
		fmt.Fprintf(&sb, "- Bits: %d\n", img.Bits)
	}
	if img.Entropy > 0 {
		fmt.Fprintf(&sb, "- Entropy: %.2f bits per byte\n", img.Entropy)
	}
	for _, k := range slices.Sorted(maps.Keys(img.Notes)) {
		fmt.Fprintf(&sb, "- %s: %s\n", k, img.Notes[k])
	}
	fmt.Fprintf(&sb, "- Sections: %d, %s: %d, exports: %d, files: %d\n", len(img.Sections), strings.ToLower(libraryLabel(img.Format)), len(img.Libraries), len(img.Exports), len(img.Files))
	if img.Truncated {
		sb.WriteString("- The lists were cut by the inspector; they are not complete.\n")
	}
	sb.WriteString("\nSee sections.md, exports.md, files.md, strings.md and one libraries/<name>.md per dependency.\n")
	return sb.String()
}

func sectionsDoc(img *Image) string {
	var sb strings.Builder
	sb.WriteString("# Sections\n\n| Name | Address | Size | Raw size | Flags |\n|---|---|---|---|---|\n")
	for _, s := range img.Sections {
		fmt.Fprintf(&sb, "| %s | %s | %d | %d | %s |\n", s.Name, s.Address, s.Size, s.RawSize, strings.Join(s.Flags, ", "))
	}
	return sb.String()
}

// listDoc renders a heading and a bullet list, or the empty text when there is nothing to list.
func listDoc(heading, empty string, items []string) string {
	body := empty + "\n"
	if len(items) > 0 {
		body = "- " + strings.Join(items, "\n- ") + "\n"
	}
	return "# " + heading + "\n\n" + body
}

func filesDoc(img *Image) string {
	items := make([]string, 0, len(img.Files))
	for _, f := range img.Files {
		items = append(items, fmt.Sprintf("%s (%d bytes%s)", f.Path, f.Size, withPrefix(", ", f.Kind)))
	}
	return listDoc("Files", "The target holds no listed files.", items)
}

func withPrefix(prefix, s string) string {
	if s == "" {
		return ""
	}
	return prefix + s
}

func libraryDoc(label string, lib coreclient.ImageLibrary) string {
	return listDoc(fmt.Sprintf("%s: %s", label, lib.Name), "No symbols are recorded for it.", lib.Symbols)
}

// Document is one file of an analysis workspace. Edges counts the things it lists (symbols, files, sections,
// strings), which is how `retro rank` orders what to read first.
type Document struct {
	Path  string `json:"path"`
	Text  string `json:"-"`
	Edges int    `json:"edges"`
	Why   string `json:"why"`
}

// Documents are the files an analysis of img writes into its workspace, in the order they are written.
func Documents(st State, img *Image) []Document {
	docs := []Document{
		{"overview.md", overview(st, img), 0, "what the target is and where the rest is"},
		{"sections.md", sectionsDoc(img), len(img.Sections), fmt.Sprintf("%d sections", len(img.Sections))},
		{"exports.md", listDoc("Exports", "The target exports nothing.", img.Exports), len(img.Exports), fmt.Sprintf("%d exports", len(img.Exports))},
		{"files.md", filesDoc(img), len(img.Files), fmt.Sprintf("%d files", len(img.Files))},
		{"strings.md", listDoc("Strings", "No strings were extracted.", img.Strings), len(img.Strings), fmt.Sprintf("%d strings", len(img.Strings))},
		{"graph.md", graphDoc(st, img), graphEdges(img), "how the target, its dependencies and their symbols connect"},
	}
	used := map[string]int{}
	label := libraryLabel(img.Format)
	for _, lib := range img.Libraries {
		docs = append(docs, Document{filepath.Join("libraries", docName(lib.Name, used)), libraryDoc(label, lib), len(lib.Symbols),
			fmt.Sprintf("%d symbols taken from %s", len(lib.Symbols), lib.Name)})
	}
	return docs
}

// graphEdges counts the edges of graphDoc: target to library, library to symbol.
func graphEdges(img *Image) int {
	n := len(img.Libraries)
	for _, lib := range img.Libraries {
		n += len(lib.Symbols)
	}
	return n
}

// graphDoc lists the edges between the target, what it depends on and the symbols it takes, one per line.
func graphDoc(st State, img *Image) string {
	var sb strings.Builder
	sb.WriteString("# Graph\n\nEdges are `from -> to`.\n\n")
	target := filepath.Base(st.Binary)
	for _, lib := range img.Libraries {
		fmt.Fprintf(&sb, "- %s -> %s\n", target, lib.Name)
		for _, sym := range lib.Symbols {
			fmt.Fprintf(&sb, "- %s -> %s\n", lib.Name, sym)
		}
	}
	if graphEdges(img) == 0 {
		sb.WriteString("The target has no recorded dependencies.\n")
	}
	return sb.String()
}

// writeWorkspace writes the documents of an analysis under dir, replacing earlier ones of the same name.
func writeWorkspace(dir string, st State, img *Image) error {
	for _, d := range Documents(st, img) {
		if err := fsutil.WriteFileAtomic(filepath.Join(dir, d.Path), []byte(d.Text)); err != nil {
			return err
		}
	}
	return nil
}

// Bundle is the evidence bundle of an analysis: the overview, the dependencies as cross-references and the exports
// as constants, capped to the 4,000-token boundary of FEAT-003.
func Bundle(st State, img *Image) *rea.EvidenceBundle {
	libs := make([]string, 0, len(img.Libraries))
	for _, l := range img.Libraries {
		libs = append(libs, l.Name)
	}
	return rea.NewEvidenceBundle(st.Binary, reaFormat(img.Format), "<overview>", overview(st, img), "", libs, img.Exports)
}
