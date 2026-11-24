package coreclient

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"Dendrite/pkg/sidecar"
)

// ErrInspectUnsupported means the dendrite-core binary predates bin.inspect.
var ErrInspectUnsupported = errors.New("dendrite-core predates bin.inspect: run `dendrite update --dev`")

// ErrNotRecognised means the target cannot be inspected at all: it does not exist, cannot be read or is not a
// regular file or directory. A file of an unknown format is not an error: it is inspected as format "unknown".
var ErrNotRecognised = errors.New("target cannot be inspected")

// Image kinds and formats of an Image. The sidecar may add formats; a client treats an unknown one as opaque text.
const (
	KindExecutable = "executable"
	KindLibrary    = "library"
	KindObject     = "object"
	KindPackage    = "package"
	KindBundle     = "bundle"
	KindSource     = "source"
	KindData       = "data"

	FormatPE         = "pe"
	FormatELF        = "elf"
	FormatMachO      = "macho"
	FormatFatMachO   = "fat-macho"
	FormatASAR       = "asar"
	FormatZIP        = "zip"
	FormatJSBundle   = "js-bundle"
	FormatSourceTree = "source-tree"
	FormatUnknown    = "unknown"
)

// ImageSection is a section or segment of an image. Addresses are hex strings, so 64-bit values survive JSON.
type ImageSection struct {
	Name    string   `json:"name"`
	Address string   `json:"address,omitempty"`
	Size    uint64   `json:"size"`
	RawSize uint64   `json:"raw_size,omitempty"`
	Flags   []string `json:"flags,omitempty"`
}

// ImageLibrary is something the image depends on (a DLL, a shared library, a dylib, a package) and the symbols
// it takes from it, when the format says.
type ImageLibrary struct {
	Name    string   `json:"name"`
	Symbols []string `json:"symbols,omitempty"`
}

// ImageFile is one file of a package, bundle or source tree.
type ImageFile struct {
	Path string `json:"path"`
	Size uint64 `json:"size"`
	Kind string `json:"kind,omitempty"`
}

// Image is what bin.inspect says about any target, whatever its format: a PE, ELF or Mach-O image, an ASAR or
// ZIP package, a JS bundle, an unknown file or a directory of source. The answer is capped to stay under 4,000
// tokens and says so in Truncated; see docs/specs/rea-bin-inspect-contract.md.
type Image struct {
	Kind      string            `json:"kind"`
	Format    string            `json:"format"`
	Arch      string            `json:"arch,omitempty"`
	Bits      int               `json:"bits,omitempty"`
	Endian    string            `json:"endian,omitempty"`
	Entry     string            `json:"entry,omitempty"`
	Base      string            `json:"base,omitempty"`
	Sections  []ImageSection    `json:"sections,omitempty"`
	Libraries []ImageLibrary    `json:"libraries,omitempty"`
	Exports   []string          `json:"exports,omitempty"`
	Files     []ImageFile       `json:"files,omitempty"`
	Strings   []string          `json:"strings,omitempty"`
	Notes     map[string]string `json:"notes,omitempty"`
	Entropy   float64           `json:"entropy,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

// Inspect reads the file or directory at path in the dendrite-core sidecar, so Go keeps no format parser of its
// own. A sidecar that predates the method answers ErrInspectUnsupported; use FromPE with PEParse for a PE image
// meanwhile.
func (c *Client) Inspect(ctx context.Context, path string) (*Image, error) {
	return callPath[Image](ctx, c, "bin.inspect", path, ErrInspectUnsupported, ErrNotRecognised)
}

// callPath asks a sidecar method that takes one path and answers a T. -32601 (the method is missing) answers
// unsupported and -32602 (the target is refused) answers rejected with the sidecar's message.
func callPath[T any](ctx context.Context, c *Client, method, path string, unsupported, rejected error) (*T, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("a path is required")
	}
	return callParams[T](ctx, c, method, map[string]any{"path": path}, unsupported, rejected)
}

// callParams asks a sidecar method with params and answers a T, mapping the two refusals as callPath does.
func callParams[T any](ctx context.Context, c *Client, method string, params map[string]any, unsupported, rejected error) (*T, error) {
	var result T
	err := c.call(ctx, method, params, &result)
	var rpcErr *sidecar.RPCError
	switch {
	case err == nil:
		return &result, nil
	case !errors.As(err, &rpcErr):
		return nil, err
	case rpcErr.Code == -32601:
		return nil, unsupported
	case rpcErr.Code == -32602:
		return nil, fmt.Errorf("%w: %s", rejected, rpcErr.Message)
	}
	return nil, err
}

// FromPE maps what pe.parse answers to an Image, for a dendrite-core that has pe.parse but not bin.inspect.
func FromPE(p *PEInfo) *Image {
	bits := 32
	if p.PE32Plus {
		bits = 64
	}
	img := &Image{Kind: KindExecutable, Format: FormatPE, Arch: p.Machine, Bits: bits, Endian: "little",
		Entry: p.EntryPoint, Base: p.ImageBase, Exports: p.Exports, Truncated: p.Truncated,
		Notes: map[string]string{"subsystem": p.Subsystem}}
	for _, s := range p.Sections {
		img.Sections = append(img.Sections, ImageSection{Name: s.Name, Address: s.VirtualAddress, Size: s.VirtualSize, RawSize: s.RawSize, Flags: s.Flags})
	}
	for _, imp := range p.Imports {
		img.Libraries = append(img.Libraries, ImageLibrary{Name: imp.DLL, Symbols: imp.Functions})
	}
	return img
}
