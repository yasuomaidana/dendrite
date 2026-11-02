package mcpserver

import "context"

// typedProp is a JSON-schema property of the given type with a description.
func typedProp(typ, description string) map[string]interface{} {
	return map[string]interface{}{"type": typ, "description": description}
}

// listProp is a JSON-schema array property whose items have the given type.
func listProp(itemType, description string) map[string]interface{} {
	prop := typedProp("array", description)
	prop["items"] = map[string]interface{}{"type": itemType}
	return prop
}

// propsSchema is an object schema with the given properties and no required keys.
func propsSchema(props map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": props}
}

// resultOrError renders a successful result as indented JSON text, or the failure under failFormat.
func resultOrError(result interface{}, err error, failFormat string) *CallToolResult {
	if err != nil {
		return errorResult(failFormat, err)
	}
	return textResult(string(indentJSON(result)))
}

// addTool registers a tool from its name, description, input schema and handler.
func addTool(s *Server, name, description string, schema map[string]interface{}, handler func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error)) {
	s.RegisterTool(Tool{Name: name, Description: description, InputSchema: schema}, handler)
}
