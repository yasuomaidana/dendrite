package mcpserver

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
