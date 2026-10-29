package rag

import (
	"context"
	"fmt"
)

// qualityProtocolMinor is the first sidecar protocol minor (1.x) that serves each quality method.
var qualityProtocolMinor = map[string]int{"quality.loc": 10, "quality.clones": 11, "quality.rstest": 11}

// Quality runs one of the sidecar's quality gates (`quality.loc`, `quality.clones`, `quality.rstest`) over the
// project at root and decodes its report into out. params carries the gate's limits; root is added. A missing or
// too old sidecar is an error that says how to install it, never a silent skip.
func (r *Retriever) Quality(ctx context.Context, root, method string, params map[string]interface{}, out interface{}) error {
	minor, known := qualityProtocolMinor[method]
	if !known {
		return fmt.Errorf("unknown quality method %q", method)
	}
	if root == "" {
		root = "."
	}
	dir, _, err := projectRoot(root)
	if err != nil {
		return err
	}
	bin, err := r.Locator.Locate(SidecarBinary)
	if err != nil {
		return fmt.Errorf("%w: %v; %s", ErrSidecarUnavailable, err, InstallHint)
	}
	c, err := r.connect(ctx, bin)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrSidecarUnavailable, bin, err)
	}
	if protocolMinor(c.Health.Protocol) < minor {
		return fmt.Errorf("%w: %s speaks protocol %s; %s needs 1.%d or later; %s",
			ErrSidecarTooOld, c.Path, c.Health.Protocol, method, minor, InstallHint)
	}
	call := map[string]interface{}{"root": dir}
	for k, v := range params {
		call[k] = v
	}
	if err := c.Call(ctx, method, call, out); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	return nil
}
