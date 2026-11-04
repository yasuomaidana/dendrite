package mcpserver

import (
	"fmt"

	"encoding/json"
	"strings"

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

// positiveIntArg reads a whole-number argument (a JSON number or a Go int); 0 when absent or not positive.
func positiveIntArg(args map[string]interface{}, key string) int {
	switch v := args[key].(type) {
	case float64:
		if v > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	}
	return 0
}

// stringArgOr reads a string argument, using def when it is absent or empty.
func stringArgOr(args map[string]interface{}, key, def string) string {
	if v, _ := args[key].(string); v != "" {
		return v
	}
	return def
}

// floatArgOr reads a positive number argument, using def when it is absent or not positive.
func floatArgOr(args map[string]interface{}, key string, def float64) float64 {
	if v, ok := args[key].(float64); ok && v > 0 {
		return v
	}
	return def
}

// nonEmptyStrings reads a JSON array of strings, dropping empty items and items that are not text.
func nonEmptyStrings(raw interface{}) []string {
	var out []string
	for _, s := range stringList(raw) {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// trimmedStrings reads a JSON array of strings, trimmed, dropping empty items and items that are not text.
func trimmedStrings(raw interface{}) []string {
	var out []string
	for _, s := range stringList(raw) {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// boolArgOr reads a boolean argument, using def when it is absent.
func boolArgOr(args map[string]interface{}, key string, def bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return def
}
