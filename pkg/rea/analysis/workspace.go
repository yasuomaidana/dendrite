package analysis

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"Dendrite/pkg/coreclient"
	"Dendrite/pkg/fsutil"
	"Dendrite/pkg/rea"
)

var unsafeName = regexp.MustCompile(`[^a-z0-9._-]+`)

// docName makes a document name from a DLL name: lower case, safe characters only, and unique among used.
func docName(dll string, used map[string]int) string {
	base := strings.Trim(unsafeName.ReplaceAllString(strings.ToLower(dll), "_"), "._-")
	if base == "" {
		base = "dll"
	}
	n := used[base]
	used[base]++
	if n > 0 {
		base = fmt.Sprintf("%s_%d", base, n+1)
	}
	return base + ".md"
}

// overview is the first document: the facts of the headers and where the rest is.
func overview(st State, info *coreclient.PEInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# PE analysis %s: %s\n\n", st.ID, filepath.Base(st.Binary))
	fmt.Fprintf(&sb, "- Format: %s (PE32+: %t)\n- Machine: %s\n- Subsystem: %s\n", st.Format, info.PE32Plus, info.Machine, info.Subsystem)
	fmt.Fprintf(&sb, "- Entry point: %s\n- Image base: %s\n", info.EntryPoint, info.ImageBase)
	fmt.Fprintf(&sb, "- Sections: %d, imported DLLs: %d, exports: %d\n", len(info.Sections), len(info.Imports), len(info.Exports))
	if info.Truncated {
		sb.WriteString("- The import and export lists were cut by the parser; they are not complete.\n")
	}
	sb.WriteString("\nSee sections.md, exports.md and one imports/<dll>.md per imported DLL.\n")
	return sb.String()
}

func sectionsDoc(info *coreclient.PEInfo) string {
	var sb strings.Builder
	sb.WriteString("# Sections\n\n| Name | Virtual address | Virtual size | Raw size | Flags |\n|---|---|---|---|---|\n")
	for _, s := range info.Sections {
		fmt.Fprintf(&sb, "| %s | %s | %d | %d | %s |\n", s.Name, s.VirtualAddress, s.VirtualSize, s.RawSize, strings.Join(s.Flags, ", "))
	}
	return sb.String()
}

func exportsDoc(info *coreclient.PEInfo) string {
	var sb strings.Builder
	sb.WriteString("# Exports\n\n")
	if len(info.Exports) == 0 {
		sb.WriteString("The image exports nothing.\n")
	}
	for _, e := range info.Exports {
		fmt.Fprintf(&sb, "- %s\n", e)
	}
	return sb.String()
}

func importDoc(imp coreclient.PEImport) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Imports from %s\n\n", imp.DLL)
	for _, f := range imp.Functions {
		fmt.Fprintf(&sb, "- %s\n", f)
	}
	return sb.String()
}

// writeWorkspace writes the documents of an analysis under dir, replacing earlier ones of the same name.
func writeWorkspace(dir string, st State, info *coreclient.PEInfo) error {
	docs := map[string]string{
		"overview.md": overview(st, info),
		"sections.md": sectionsDoc(info),
		"exports.md":  exportsDoc(info),
	}
	used := map[string]int{}
	for _, imp := range info.Imports {
		docs[filepath.Join("imports", docName(imp.DLL, used))] = importDoc(imp)
	}
	for name, text := range docs {
		if err := fsutil.WriteFileAtomic(filepath.Join(dir, name), []byte(text)); err != nil {
			return err
		}
	}
	return nil
}

// Bundle is the evidence bundle of an analysis: the overview, the imported DLLs as cross-references and the exports
// as constants, capped to the 4,000-token boundary of FEAT-003.
func Bundle(st State, info *coreclient.PEInfo) *rea.EvidenceBundle {
	dlls := make([]string, 0, len(info.Imports))
	for _, imp := range info.Imports {
		dlls = append(dlls, imp.DLL)
	}
	return rea.NewEvidenceBundle(st.Binary, rea.FormatPE, "<pe overview>", overview(st, info), "", dlls, info.Exports)
}
