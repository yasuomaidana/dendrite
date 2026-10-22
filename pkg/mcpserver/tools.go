package mcpserver

import (
	"Dendrite/pkg/tracker/plane"
)

// Dependencies holds external resources required for tool execution.
type Dependencies struct {
	PlaneClient *plane.Client
	DevStatus   string
	OllamaURL   string
}
