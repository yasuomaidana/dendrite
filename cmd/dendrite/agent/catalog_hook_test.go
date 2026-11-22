package agent

import (
	"context"
	"io"
)

// Tests must not ask the real gateway, Ollama or providers for their models.
func init() { refreshCatalog = func(context.Context, io.Writer) {} }
