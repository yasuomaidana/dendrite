package coreclient

import (
	"context"
	"errors"
	"strings"
)

// ErrFunctionsUnsupported means the dendrite-core binary predates bin.functions.
var ErrFunctionsUnsupported = errors.New("dendrite-core predates bin.functions: run `dendrite update --dev`")

// ErrNoFunctions means the target has no function list the sidecar can build: it is not a PE image, its format
// or machine is not supported, or it is cut short.
var ErrNoFunctions = errors.New("the functions of this target cannot be listed")

// Function is one function the sidecar found. Addresses are hex strings, so 64-bit values survive JSON. Name is
// nil when the image does not name it. Cyclomatic is computed from the control-flow graph.
type Function struct {
	Start        string  `json:"start"`
	End          string  `json:"end"`
	Size         uint64  `json:"size"`
	Name         *string `json:"name"`
	Cyclomatic   int     `json:"cyclomatic"`
	Address      string  `json:"address,omitempty"`
	RVA          string  `json:"rva,omitempty"`
	Blocks       int     `json:"blocks,omitempty"`
	Instructions int     `json:"instructions,omitempty"`
	// Partial says the walk of this function spent its budget, so Cyclomatic is a lower bound.
	Partial bool `json:"partial,omitempty"`
}

// FunctionPage is one page of bin.functions: at most 48 functions ordered by RVA, and where the next page starts.
type FunctionPage struct {
	Format     string     `json:"format"`
	Arch       string     `json:"arch"`
	Total      int        `json:"total"`
	Offset     int        `json:"offset"`
	NextOffset *int       `json:"next_offset"`
	Truncated  bool       `json:"truncated"`
	Functions  []Function `json:"functions"`
}

// FunctionList is every function of a target, the pages joined. Truncated says the sidecar spent an analysis
// budget, so some functions may be missing or their complexity undercounted.
type FunctionList struct {
	Format    string
	Arch      string
	Total     int
	Truncated bool
	Functions []Function
}

// FunctionsPage reads one page of the functions of the target at path, from offset, keeping only those whose
// cyclomatic complexity is at least minCyclomatic.
func (c *Client) FunctionsPage(ctx context.Context, path string, offset, minCyclomatic int) (*FunctionPage, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("a path is required")
	}
	params := map[string]any{"path": path, "offset": offset, "min_cyclomatic": minCyclomatic}
	return callParams[FunctionPage](ctx, c, "bin.functions", params, ErrFunctionsUnsupported, ErrNoFunctions)
}

// Functions reads every page of the functions of the target at path. A page that points backwards or past the
// total ends the walk, so a misbehaving sidecar cannot hold the caller for good.
func (c *Client) Functions(ctx context.Context, path string) (*FunctionList, error) {
	list := &FunctionList{}
	for offset := 0; ; {
		got, err := c.FunctionsPage(ctx, path, offset, 0)
		if err != nil {
			return nil, err
		}
		list.Format, list.Arch, list.Total = got.Format, got.Arch, got.Total
		list.Truncated = list.Truncated || got.Truncated
		list.Functions = append(list.Functions, got.Functions...)
		if got.NextOffset == nil || *got.NextOffset <= offset || *got.NextOffset >= got.Total {
			return list, nil
		}
		offset = *got.NextOffset
	}
}
