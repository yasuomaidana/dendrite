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

// IndentJSON renders v as indented JSON. Tool results are plain data (structs, maps and slices of
// strings and numbers), which always marshal.
func IndentJSON(v interface{}) []byte {
	data, _ := json.MarshalIndent(v, "", "  ")
	return data
}

// TextResult is a successful tool result holding one text item.
func TextResult(text string) *CallToolResult {
	return &CallToolResult{Content: []ContentItem{{Type: "text", Text: text}}}
}

// PlaneClientOf is the Plane client the tools were given, otherwise one built from the environment.
func PlaneClientOf(deps Dependencies) (*plane.Client, error) {
	if deps.PlaneClient != nil {
		return deps.PlaneClient, nil
	}
	client, err := plane.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("plane client error: %w", err)
	}
	return client, nil
}

// PositiveIntArg reads a whole-number argument (a JSON number or a Go int); 0 when absent or not positive.
func PositiveIntArg(args map[string]interface{}, key string) int {
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

// StringArgOr reads a string argument, using def when it is absent or empty.
func StringArgOr(args map[string]interface{}, key, def string) string {
	if v, _ := args[key].(string); v != "" {
		return v
	}
	return def
}

// FloatArgOr reads a positive number argument, using def when it is absent or not positive.
func FloatArgOr(args map[string]interface{}, key string, def float64) float64 {
	if v, ok := args[key].(float64); ok && v > 0 {
		return v
	}
	return def
}

// NonEmptyStrings reads a JSON array of strings, dropping empty items and items that are not text.
func NonEmptyStrings(raw interface{}) []string {
	var out []string
	for _, s := range StringList(raw) {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// TrimmedStrings reads a JSON array of strings, trimmed, dropping empty items and items that are not text.
func TrimmedStrings(raw interface{}) []string {
	var out []string
	for _, s := range StringList(raw) {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// BoolArgOr reads a boolean argument, using def when it is absent.
func BoolArgOr(args map[string]interface{}, key string, def bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return def
}

// StringList reads a JSON array of strings; items that are not text are skipped.
func StringList(raw interface{}) []string {
	var out []string
	items, _ := raw.([]interface{})
	for _, o := range items {
		if s, ok := o.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
