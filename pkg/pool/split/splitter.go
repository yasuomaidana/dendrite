package split

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"Dendrite/pkg/pool/dag"
	"Dendrite/pkg/pool/switcher"
	"Dendrite/pkg/ui/modal"
)

// ProjectSplitter coordinates extraction of AST slices and creation of new pool members.
type ProjectSplitter struct {
	switcher *switcher.DependencySwitcher
}

// NewProjectSplitter creates a new ProjectSplitter instance.
func NewProjectSplitter() *ProjectSplitter {
	return &ProjectSplitter{
		switcher: switcher.NewDependencySwitcher(),
	}
}

// Split extracts an AST slice into a new project, enforcing the target directory gate.
func (s *ProjectSplitter) Split(ctx context.Context, req SplitRequest) (*SplitResult, error) {
	// 1. Verify source project exists
	absSource, err := filepath.Abs(req.SourceProjectDir)
	if err != nil {
		return nil, fmt.Errorf("resolving source directory: %w", err)
	}
	if !dirExists(absSource) {
		return nil, ErrSourceProjectNotFound
	}

	// 2. Verify module subpath exists
	sourceModulePath := filepath.Join(absSource, req.ModuleSubpath)
	if !dirExists(sourceModulePath) {
		return nil, ErrModuleNotFound
	}

	if strings.TrimSpace(req.NewMemberName) == "" {
		return nil, fmt.Errorf("new_member_name cannot be empty")
	}

	// 3. Mandatory Target Directory Gate
	targetDir := strings.TrimSpace(req.TargetDirectory)
	if targetDir == "" {
		if req.Prompt == nil {
			return nil, ErrTargetDirRequired
		}
		picker := modal.NewDirectoryPickerModal(req.NewMemberName, filepath.Base(absSource), "")
		selected, err := req.Prompt(picker)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSplitAborted, err)
		}
		targetDir = strings.TrimSpace(selected)
		if targetDir == "" {
			return nil, ErrTargetDirRequired
		}
	}

	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("resolving target directory: %w", err)
	}

	// Check if target directory already exists and has files
	if dirExists(absTarget) {
		entries, err := os.ReadDir(absTarget)
		if err == nil && len(entries) > 0 {
			return nil, ErrTargetDirNotEmpty
		}
	}

	// 4. Begin extraction with rollback guard
	rollback := true
	var createdFiles []string
	defer func() {
		if rollback {
			for _, f := range createdFiles {
				_ = os.Remove(f)
			}
			_ = os.RemoveAll(absTarget)
		}
	}()

	if err := os.MkdirAll(absTarget, 0755); err != nil {
		return nil, fmt.Errorf("creating target directory: %w", err)
	}

	// 5. Copy module files & tests
	extractedFiles, err := copyDirectory(sourceModulePath, absTarget)
	if err != nil {
		return nil, fmt.Errorf("extracting module files: %w", err)
	}
	createdFiles = append(createdFiles, extractedFiles...)

	// 6. Scaffold manifest in target directory
	manifestName, manifestPath, err := scaffoldManifest(absSource, absTarget, req.NewMemberName)
	if err != nil {
		return nil, fmt.Errorf("scaffolding manifest: %w", err)
	}
	createdFiles = append(createdFiles, manifestPath)

	// 7. Wire local mode in parent if requested
	wiredLocal := false
	if req.WireLocalMode {
		switchReq := switcher.SwitchRequest{
			TargetProjectDir: absSource,
			DependencyName:   req.NewMemberName,
			Mode:             switcher.ModeLocal,
			LocalPath:        absTarget,
			SyncIDE:          true,
		}
		_, err := s.switcher.Switch(ctx, switchReq)
		if err == nil {
			wiredLocal = true
		}
	}

	// 8. Register in pool if requested
	poolRegistered := false
	if req.RegisterInPool {
		pm := dag.NewPoolManager(req.PoolConfigFile)
		if _, err := pm.Add(absTarget); err == nil {
			_ = pm.Save()
			poolRegistered = true
		}
	}

	// Successful completion; disarm rollback
	rollback = false

	return &SplitResult{
		SourceProject:    absSource,
		NewMemberName:    req.NewMemberName,
		TargetDirectory:  absTarget,
		ExtractedFiles:   extractedFiles,
		ManifestCreated:  manifestName,
		ParentWiredLocal: wiredLocal,
		PoolRegistered:   poolRegistered,
		Timestamp:        time.Now().UTC(),
	}, nil
}

func copyDirectory(src, dst string) ([]string, error) {
	var copied []string
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, data, info.Mode()); err != nil {
			return err
		}
		copied = append(copied, targetPath)
		return nil
	})
	return copied, err
}

func scaffoldManifest(parentDir, targetDir, memberName string) (string, string, error) {
	if fileExists(filepath.Join(parentDir, "go.mod")) {
		p := filepath.Join(targetDir, "go.mod")
		content := fmt.Sprintf("module %s\n\ngo 1.24\n", memberName)
		return "go.mod", p, os.WriteFile(p, []byte(content), 0644)
	}

	if fileExists(filepath.Join(parentDir, "Cargo.toml")) {
		p := filepath.Join(targetDir, "Cargo.toml")
		content := fmt.Sprintf("[package]\nname = \"%s\"\nversion = \"0.1.0\"\nedition = \"2024\"\n", memberName)
		return "Cargo.toml", p, os.WriteFile(p, []byte(content), 0644)
	}

	if fileExists(filepath.Join(parentDir, "package.json")) {
		p := filepath.Join(targetDir, "package.json")
		content := fmt.Sprintf("{\n  \"name\": \"%s\",\n  \"version\": \"0.1.0\"\n}\n", memberName)
		return "package.json", p, os.WriteFile(p, []byte(content), 0644)
	}

	if fileExists(filepath.Join(parentDir, "pyproject.toml")) {
		p := filepath.Join(targetDir, "pyproject.toml")
		content := fmt.Sprintf("[project]\nname = \"%s\"\nversion = \"0.1.0\"\n", memberName)
		return "pyproject.toml", p, os.WriteFile(p, []byte(content), 0644)
	}

	// Fallback Go manifest
	p := filepath.Join(targetDir, "go.mod")
	content := fmt.Sprintf("module %s\n\ngo 1.24\n", memberName)
	return "go.mod", p, os.WriteFile(p, []byte(content), 0644)
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Suppress unused import warnings
var _ = io.EOF
