package docgen

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// RenderFeatureSpec generates valid Markdown for a FEAT-* specification with mandatory YAML frontmatter.
func RenderFeatureSpec(spec FeatureSpec) (string, error) {
	if strings.TrimSpace(spec.ID) == "" {
		return "", ErrEmptySpecID
	}
	if strings.TrimSpace(spec.Title) == "" {
		return "", ErrEmptyTitle
	}

	status := spec.Status
	if status == "" {
		status = "approved"
	}
	repo := spec.Repository
	if repo == "" {
		repo = "dendrite"
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", spec.ID))
	sb.WriteString(fmt.Sprintf("title: \"%s\"\n", spec.Title))
	sb.WriteString(fmt.Sprintf("status: %s\n", status))
	if spec.PlaneEpic != "" {
		sb.WriteString(fmt.Sprintf("plane_epic: %s\n", spec.PlaneEpic))
	}
	if len(spec.Dependencies) > 0 {
		sb.WriteString("dependencies:\n")
		for _, dep := range spec.Dependencies {
			sb.WriteString(fmt.Sprintf("  - %s\n", dep))
		}
	} else {
		sb.WriteString("dependencies: []\n")
	}
	sb.WriteString(fmt.Sprintf("repository: %s\n", repo))
	if len(spec.Tags) > 0 {
		sb.WriteString("tags: [")
		sb.WriteString(strings.Join(spec.Tags, ", "))
		sb.WriteString("]\n")
	}
	sb.WriteString("---\n\n")

	sb.WriteString(fmt.Sprintf("# %s: %s\n\n", spec.ID, spec.Title))

	if spec.UserStory != "" {
		sb.WriteString("## 1. User Story & Business Objective\n")
		sb.WriteString(spec.UserStory + "\n\n---\n\n")
	}

	if len(spec.TechnicalBoundaries) > 0 {
		sb.WriteString("## 2. Technical Boundaries & Specifications\n\n")
		for i, b := range spec.TechnicalBoundaries {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, b))
		}
		sb.WriteString("\n---\n\n")
	}

	if len(spec.AcceptanceCriteria) > 0 {
		sb.WriteString("## 3. Acceptance Criteria & Verification Matrix\n\n")
		for _, ac := range spec.AcceptanceCriteria {
			sb.WriteString(fmt.Sprintf("- [x] %s\n", ac))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// RenderADRRecord generates valid Markdown for an ADR-* specification with mandatory YAML frontmatter.
func RenderADRRecord(adr ADRRecord) (string, error) {
	if strings.TrimSpace(adr.ID) == "" {
		return "", ErrEmptySpecID
	}
	if strings.TrimSpace(adr.Title) == "" {
		return "", ErrEmptyTitle
	}

	dateStr := adr.Date
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}
	status := adr.Status
	if status == "" {
		status = "proposed"
	}
	repo := adr.Repository
	if repo == "" {
		repo = "dendrite"
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", adr.ID))
	sb.WriteString(fmt.Sprintf("title: \"%s\"\n", adr.Title))
	sb.WriteString(fmt.Sprintf("status: %s\n", status))
	sb.WriteString(fmt.Sprintf("date: %s\n", dateStr))
	sb.WriteString(fmt.Sprintf("repository: %s\n", repo))
	if len(adr.Tags) > 0 {
		sb.WriteString("tags: [")
		sb.WriteString(strings.Join(adr.Tags, ", "))
		sb.WriteString("]\n")
	}
	sb.WriteString("---\n\n")

	sb.WriteString(fmt.Sprintf("# %s: %s\n\n", adr.ID, adr.Title))
	sb.WriteString("## 1. Context & Problem Statement\n")
	sb.WriteString(adr.Context + "\n\n")

	sb.WriteString("## 2. Decision\n")
	sb.WriteString(adr.Decision + "\n\n")

	sb.WriteString("## 3. Consequences\n")
	sb.WriteString(adr.Consequences + "\n")

	return sb.String(), nil
}

// GenerateOpenAPI31 compiles a list of endpoints into a valid OpenAPI 3.1.0 JSON document.
func GenerateOpenAPI31(title, version string, endpoints []OpenAPIEndpoint) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", ErrEmptyTitle
	}
	if strings.TrimSpace(version) == "" {
		return "", ErrEmptyVersion
	}

	paths := make(map[string]PathItem)

	for _, ep := range endpoints {
		path := ep.Path
		if path == "" {
			path = "/"
		}
		method := strings.ToLower(ep.Method)
		if method == "" {
			method = "get"
		}

		responses := make(map[string]OperationResponse)
		for code, desc := range ep.Responses {
			responses[code] = OperationResponse{Description: desc}
		}
		if len(responses) == 0 {
			responses["200"] = OperationResponse{Description: "Successful operation"}
		}

		op := Operation{
			Summary:     ep.Summary,
			Description: ep.Description,
			Responses:   responses,
		}

		if _, exists := paths[path]; !exists {
			paths[path] = make(PathItem)
		}
		paths[path][method] = op
	}

	spec := OpenAPISpec{
		OpenAPI: "3.1.0",
		Info: OpenAPIInfo{
			Title:   title,
			Version: version,
		},
		Paths: paths,
	}

	bytes, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// ExtractEndpointsFromSource identifies HTTP endpoints from router registration comments or patterns.
func ExtractEndpointsFromSource(source string) []OpenAPIEndpoint {
	var endpoints []OpenAPIEndpoint
	scanner := bufio.NewScanner(strings.NewReader(source))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Match patterns like: // @Route GET /api/v1/health [Get health status]
		if strings.HasPrefix(line, "// @Route ") {
			parts := strings.Fields(strings.TrimPrefix(line, "// @Route "))
			if len(parts) >= 2 {
				method := parts[0]
				path := parts[1]
				summary := strings.Join(parts[2:], " ")
				endpoints = append(endpoints, OpenAPIEndpoint{
					Method:  method,
					Path:    path,
					Summary: summary,
					Responses: map[string]string{
						"200": "OK",
					},
				})
			}
		}
	}
	return endpoints
}
