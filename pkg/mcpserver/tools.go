package mcpserver

import (
	"fmt"

	"encoding/json"

	"Dendrite/pkg/tracker/plane"
)

// Dependencies holds external resources required for tool execution.
type Dependencies struct {
	PlaneClient *plane.Client
	DevStatus   string
	OllamaURL   string
}

// indentJSON renders v as indented JSON. Tool results are plain data (structs, maps and slices of
// strings and numbers), which always marshal.
func indentJSON(v interface{}) []byte {
	data, _ := json.MarshalIndent(v, "", "  ")
	return data
}

// textResult is a successful tool result holding one text item.
func textResult(text string) *CallToolResult {
	return &CallToolResult{Content: []ContentItem{{Type: "text", Text: text}}}
}

// planeClientOf is the Plane client the tools were given, otherwise one built from the environment.
func planeClientOf(deps Dependencies) (*plane.Client, error) {
	if deps.PlaneClient != nil {
		return deps.PlaneClient, nil
	}
	client, err := plane.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("plane client error: %w", err)
	}
	return client, nil
}
