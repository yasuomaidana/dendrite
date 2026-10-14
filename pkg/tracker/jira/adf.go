package jira

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ADFNode represents a generic Atlassian Document Format node.
type ADFNode struct {
	Type    string                 `json:"type"`
	Attrs   map[string]interface{} `json:"attrs,omitempty"`
	Content []ADFNode              `json:"content,omitempty"`
	Text    string                 `json:"text,omitempty"`
	Marks   []ADFMark              `json:"marks,omitempty"`
}

// ADFMark represents an inline style markup (e.g. bold, code, em, link).
type ADFMark struct {
	Type  string                 `json:"type"`
	Attrs map[string]interface{} `json:"attrs,omitempty"`
}

// ParseADFToMarkdown parses an ADF JSON representation into clean markdown.
func ParseADFToMarkdown(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// If raw is just a JSON string, return unquoted string
	var strVal string
	if err := json.Unmarshal(raw, &strVal); err == nil {
		return strings.TrimSpace(strVal)
	}

	var root ADFNode
	if err := json.Unmarshal(raw, &root); err != nil {
		return string(raw)
	}

	var sb strings.Builder
	renderNode(&sb, root, 0)
	return strings.TrimSpace(sb.String())
}

func renderNode(sb *strings.Builder, node ADFNode, listDepth int) {
	switch node.Type {
	case "doc":
		for _, child := range node.Content {
			renderNode(sb, child, listDepth)
			sb.WriteString("\n\n")
		}

	case "paragraph":
		for _, child := range node.Content {
			renderNode(sb, child, listDepth)
		}

	case "heading":
		level := 1
		if l, ok := node.Attrs["level"].(float64); ok && l >= 1 && l <= 6 {
			level = int(l)
		}
		sb.WriteString(strings.Repeat("#", level))
		sb.WriteString(" ")
		for _, child := range node.Content {
			renderNode(sb, child, listDepth)
		}

	case "text":
		text := node.Text
		for _, mark := range node.Marks {
			switch mark.Type {
			case "strong":
				text = fmt.Sprintf("**%s**", text)
			case "em":
				text = fmt.Sprintf("*%s*", text)
			case "code":
				text = fmt.Sprintf("`%s`", text)
			case "link":
				if href, ok := mark.Attrs["href"].(string); ok {
					text = fmt.Sprintf("[%s](%s)", text, href)
				}
			}
		}
		sb.WriteString(text)

	case "bulletList":
		for _, item := range node.Content {
			indent := strings.Repeat("  ", listDepth)
			sb.WriteString(indent + "- ")
			for _, child := range item.Content {
				renderNode(sb, child, listDepth+1)
			}
			sb.WriteString("\n")
		}

	case "orderedList":
		for i, item := range node.Content {
			indent := strings.Repeat("  ", listDepth)
			sb.WriteString(fmt.Sprintf("%s%d. ", indent, i+1))
			for _, child := range item.Content {
				renderNode(sb, child, listDepth+1)
			}
			sb.WriteString("\n")
		}

	case "codeBlock":
		lang := ""
		if l, ok := node.Attrs["language"].(string); ok {
			lang = l
		}
		sb.WriteString(fmt.Sprintf("```%s\n", lang))
		for _, child := range node.Content {
			renderNode(sb, child, listDepth)
		}
		sb.WriteString("\n```")

	case "blockquote":
		sb.WriteString("> ")
		for _, child := range node.Content {
			renderNode(sb, child, listDepth)
		}

	default:
		for _, child := range node.Content {
			renderNode(sb, child, listDepth)
		}
	}
}

// BuildADFFromMarkdown creates a basic ADF JSON doc from plain markdown text for comments/updates.
func BuildADFFromMarkdown(text string) map[string]interface{} {
	paragraphs := strings.Split(text, "\n\n")
	var content []map[string]interface{}

	for _, p := range paragraphs {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		content = append(content, map[string]interface{}{
			"type": "paragraph",
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": trimmed,
				},
			},
		})
	}

	return map[string]interface{}{
		"type":    "doc",
		"version": 1,
		"content": content,
	}
}
