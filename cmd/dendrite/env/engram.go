package env

import (
	"context"
	"fmt"
	"io"
	"time"

	"Dendrite/pkg/engram"
)

// diagnoseEngram reports whether the Engram agent-memory binary is installed and what its own doctor says.
func diagnoseEngram(ctx context.Context, out io.Writer) {
	bin, err := engram.Locate()
	if err != nil {
		fmt.Fprintf(out, "  [-] Engram: %v\n", err)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	version, err := engram.Version(ctx, bin)
	if err != nil {
		fmt.Fprintf(out, "  [-] Engram: %s found but does not run (%v)\n", bin, err)
		return
	}
	h, err := engram.Doctor(ctx, bin)
	if err != nil {
		fmt.Fprintf(out, "  [-] Engram: %s at %s, doctor failed (%v)\n", version, bin, err)
		return
	}
	mark := "✓"
	if h.Errors > 0 || h.Blocked > 0 {
		mark = "-"
	}
	fmt.Fprintf(out, "  [%s] Engram: %s at %s, doctor %s (%d/%d checks ok)\n", mark, version, bin, h.Status, h.OK, h.Total)
}
