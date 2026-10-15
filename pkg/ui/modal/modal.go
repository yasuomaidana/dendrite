package modal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Standard typed errors for directory picker modal.
var (
	ErrEmptyDirectoryPath = errors.New("directory path cannot be empty")
	ErrPromptCancelled    = errors.New("interactive target directory prompt was cancelled by user")
)

// DirectoryPickerModal encapsulates interactive directory confirmation for CLI and Web UI.
type DirectoryPickerModal struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	MemberName     string   `json:"member_name"`
	SourceProject  string   `json:"source_project"`
	InferredEdge   string   `json:"inferred_edge"`
	DefaultPath    string   `json:"default_path,omitempty"`
	SuggestedPaths []string `json:"suggested_paths,omitempty"`
	Required       bool     `json:"required"`
}

// NewDirectoryPickerModal builds a new modal descriptor for a member split operation.
func NewDirectoryPickerModal(memberName, sourceProject, defaultPath string) *DirectoryPickerModal {
	return &DirectoryPickerModal{
		Title:         fmt.Sprintf("Target Directory Selection: %s", memberName),
		Description:   "Extracting AST slice and scaffolding autonomous pool member project.",
		MemberName:    memberName,
		SourceProject: sourceProject,
		InferredEdge:  fmt.Sprintf("%s -> %s", sourceProject, memberName),
		DefaultPath:   defaultPath,
		Required:      true,
	}
}

// RenderCLI executes the interactive prompt on terminal stdin/stdout.
func (m *DirectoryPickerModal) RenderCLI(r io.Reader, w io.Writer) (string, error) {
	fmt.Fprintf(w, "\n[i] Inferred Dependency Edge: %s\n", m.InferredEdge)
	promptText := fmt.Sprintf("? Specify the target directory for %s (required): ", m.MemberName)
	if m.DefaultPath != "" {
		promptText = fmt.Sprintf("? Specify the target directory for %s [%s] (required): ", m.MemberName, m.DefaultPath)
	}
	fmt.Fprint(w, promptText)

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", ErrPromptCancelled
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		if m.DefaultPath != "" {
			return m.DefaultPath, nil
		}
		return "", ErrEmptyDirectoryPath
	}

	if strings.EqualFold(input, "cancel") || strings.EqualFold(input, "q") {
		return "", ErrPromptCancelled
	}

	return input, nil
}

// RenderJSON serializes the modal for the Web UI WebSocket / HTTP modal overlay.
func (m *DirectoryPickerModal) RenderJSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}
