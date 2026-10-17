package reflect

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"Dendrite/pkg/mcpserver"
)

// MCPToolDoc holds structured documentation data for an MCP tool.
type MCPToolDoc struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
	ExampleArgs map[string]interface{} `json:"example_args"`
}

// IntrospectMCPTools introspects a list of MCP tools and returns structured documentation models sorted by name.
func IntrospectMCPTools(tools []mcpserver.Tool) []MCPToolDoc {
	sortedTools := append([]mcpserver.Tool(nil), tools...)
	sort.Slice(sortedTools, func(i, j int) bool {
		return sortedTools[i].Name < sortedTools[j].Name
	})

	docs := make([]MCPToolDoc, 0, len(sortedTools))
	for _, t := range sortedTools {
		docs = append(docs, IntrospectMCPTool(t))
	}
	return docs
}

// IntrospectMCPServer introspects an MCP server and extracts all tools as documentation models.
func IntrospectMCPServer(srv *mcpserver.Server) []MCPToolDoc {
	if srv == nil {
		return nil
	}
	return IntrospectMCPTools(srv.ListTools())
}

// IntrospectMCPTool introspects a single MCP tool and synthesizes example call arguments.
func IntrospectMCPTool(tool mcpserver.Tool) MCPToolDoc {
	exampleArgs := synthesizeExampleArguments(tool.InputSchema)
	return MCPToolDoc{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: tool.InputSchema,
		ExampleArgs: exampleArgs,
	}
}

// RenderMCPToolMarkdown generates the markdown documentation string for an MCP tool.
func RenderMCPToolMarkdown(doc MCPToolDoc) string {
	var sb strings.Builder

	// Frontmatter
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", doc.Name))
	sb.WriteString(fmt.Sprintf("title: \"%s\"\n", doc.Name))
	sb.WriteString(fmt.Sprintf("sidebar_label: \"%s\"\n", doc.Name))
	sb.WriteString("---\n\n")

	// Heading & Description
	sb.WriteString(fmt.Sprintf("# %s\n\n", doc.Name))
	if doc.Description != "" {
		sb.WriteString(doc.Description + "\n\n")
	} else {
		sb.WriteString("No description available.\n\n")
	}

	// JSON Schema
	sb.WriteString("## Input Schema\n\n")
	sb.WriteString("```json\n")
	schemaBytes, err := json.MarshalIndent(doc.InputSchema, "", "  ")
	if err == nil && len(schemaBytes) > 0 {
		sb.WriteString(string(schemaBytes))
	} else {
		sb.WriteString("{\n  \"type\": \"object\",\n  \"properties\": {}\n}")
	}
	sb.WriteString("\n```\n\n")

	// Example Call
	sb.WriteString("## Example Call\n\n")
	sb.WriteString("### JSON-RPC\n\n")
	sb.WriteString("```json\n")
	callPayload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      doc.Name,
			"arguments": doc.ExampleArgs,
		},
	}
	callBytes, _ := json.MarshalIndent(callPayload, "", "  ")
	sb.WriteString(string(callBytes))
	sb.WriteString("\n```\n\n")

	sb.WriteString("### CLI\n\n")
	sb.WriteString("```bash\n")
	argBytes, _ := json.Marshal(doc.ExampleArgs)
	if string(argBytes) == "{}" {
		sb.WriteString(fmt.Sprintf("dendrite mcp call %s\n", doc.Name))
	} else {
		sb.WriteString(fmt.Sprintf("dendrite mcp call %s '%s'\n", doc.Name, string(argBytes)))
	}
	sb.WriteString("```\n")

	return sb.String()
}

func synthesizeExampleArguments(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return make(map[string]interface{})
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok || len(props) == 0 {
		return make(map[string]interface{})
	}

	result := make(map[string]interface{})
	for key, val := range props {
		propMap, isMap := val.(map[string]interface{})
		if !isMap {
			result[key] = "example"
			continue
		}
		propType, _ := propMap["type"].(string)
		desc, _ := propMap["description"].(string)

		switch propType {
		case "string":
			if strings.Contains(desc, "default:") {
				idx := strings.Index(desc, "default:")
				def := strings.Trim(strings.TrimSpace(desc[idx+len("default:"):len(desc)]), `()"`)
				result[key] = def
			} else if strings.Contains(key, "ticket") {
				result[key] = "DEN-985"
			} else if strings.Contains(key, "file") || strings.Contains(key, "path") {
				result[key] = "DEV_STATUS.md"
			} else if strings.Contains(key, "status") {
				result[key] = "DONE"
			} else {
				result[key] = "sample"
			}
		case "boolean":
			result[key] = false
		case "integer", "number":
			result[key] = 1
		case "array":
			result[key] = []string{"item1"}
		case "object":
			result[key] = map[string]interface{}{}
		default:
			result[key] = "value"
		}
	}
	return result
}
