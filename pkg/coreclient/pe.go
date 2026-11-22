package coreclient

import (
	"context"
	"errors"
)

// ErrPEUnsupported means the dendrite-core binary predates pe.parse.
var ErrPEUnsupported = errors.New("dendrite-core predates pe.parse: run `dendrite update --dev`")

// ErrNotPE means the file is not a PE image (bad DOS or PE signature, or cut short).
var ErrNotPE = errors.New("not a PE file")

// PESection is one section of a PE image. Addresses are hex strings, so 64-bit values survive JSON.
type PESection struct {
	Name           string   `json:"name"`
	VirtualAddress string   `json:"virtual_address"`
	VirtualSize    uint64   `json:"virtual_size"`
	RawSize        uint64   `json:"raw_size"`
	Flags          []string `json:"flags"`
}

// PEImport is the functions one DLL provides to the image.
type PEImport struct {
	DLL       string   `json:"dll"`
	Functions []string `json:"functions"`
}

// PEInfo is the structure of a PE32 or PE32+ image: headers, sections, imports and exports. The sidecar caps the
// answer (one budget of 500 names for DLLs, imported functions and exports, 96 sections) and says so in Truncated;
// see docs/specs/rea-pe-parse-contract.md.
type PEInfo struct {
	Machine    string      `json:"machine"`
	Subsystem  string      `json:"subsystem"`
	PE32Plus   bool        `json:"pe32_plus"`
	EntryPoint string      `json:"entry_point"`
	ImageBase  string      `json:"image_base"`
	Sections   []PESection `json:"sections"`
	Imports    []PEImport  `json:"imports"`
	Exports    []string    `json:"exports"`
	Truncated  bool        `json:"truncated"`
}

// PEParse reads the PE image at path in the dendrite-core sidecar, so Go keeps no PE parser of its own.
func (c *Client) PEParse(ctx context.Context, path string) (*PEInfo, error) {
	var info PEInfo
	if err := c.callPath(ctx, "pe.parse", path, &info, ErrPEUnsupported, ErrNotPE); err != nil {
		return nil, err
	}
	return &info, nil
}
