package mesh

import (
	"context"
	"io"
)

// Tests must not ask the real gateway, Ollama or providers for their models.
func init() { refreshCatalogIfStale = func(context.Context, io.Writer) {} }
