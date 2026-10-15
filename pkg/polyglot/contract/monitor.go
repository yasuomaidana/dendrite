package contract

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"Dendrite/pkg/jetbrains"
)

// ContractMonitor scans shared schema repositories and models across Quad-IDE projects.
type ContractMonitor struct {
	bridge *jetbrains.Bridge
}

// NewContractMonitor initializes a new polyglot contract drift monitor.
func NewContractMonitor(bridge *jetbrains.Bridge) *ContractMonitor {
	return &ContractMonitor{bridge: bridge}
}

// ScanSchemas reads and parses schema specifications from one or more directories.
func (m *ContractMonitor) ScanSchemas(schemaDirs []string) ([]ContractSchema, error) {
	var results []ContractSchema

	for _, dir := range schemaDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			content := string(data)

			switch ext {
			case ".proto":
				schemas, _ := ParseProtobuf(content, path)
				results = append(results, schemas...)
			case ".json":
				// Could be JSONSchema or OpenAPI JSON
				if strings.Contains(content, `"openapi"`) || strings.Contains(content, `"swagger"`) {
					schemas, _ := ParseOpenAPI(content, path)
					results = append(results, schemas...)
				} else if strings.Contains(content, `"properties"`) || strings.Contains(content, `"$schema"`) {
					schemas, _ := ParseJSONSchema(content, path)
					results = append(results, schemas...)
				}
			case ".yaml", ".yml":
				if strings.Contains(content, "openapi:") || strings.Contains(content, "swagger:") {
					schemas, _ := ParseOpenAPI(content, path)
					results = append(results, schemas...)
				}
			}
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("walking schema dir %s: %w", dir, err)
		}
	}

	if len(results) == 0 {
		return nil, ErrNoSchemasFound
	}

	return results, nil
}

// ScanCodeModels scans language projects and extracts implementation schemas.
func (m *ContractMonitor) ScanCodeModels(codeDirs map[TargetLanguage]string) ([]ContractSchema, error) {
	var results []ContractSchema

	for lang, dir := range codeDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			content := string(data)

			switch lang {
			case LangGo:
				if ext == ".go" && !strings.HasSuffix(path, "_test.go") {
					schemas, _ := ParseGoStructs(content, path)
					results = append(results, schemas...)
				}
			case LangRust:
				if ext == ".rs" {
					schemas, _ := ParseRustSerdeStructs(content, path)
					results = append(results, schemas...)
				}
			case LangTypeScript:
				if ext == ".ts" || ext == ".tsx" {
					schemas, _ := ParseTypeScriptInterfaces(content, path)
					results = append(results, schemas...)
				}
			case LangPython:
				if ext == ".py" {
					schemas, _ := ParsePythonPydanticModels(content, path)
					results = append(results, schemas...)
				}
			}
			return nil
		})
	}

	return results, nil
}

// CheckDrift runs end-to-end drift detection across schemas and Quad-IDE codebases.
func (m *ContractMonitor) CheckDrift(schemaDirs []string, codeDirs map[TargetLanguage]string) (*DriftReport, error) {
	canonicalSchemas, err := m.ScanSchemas(schemaDirs)
	if err != nil {
		return nil, err
	}

	codeSchemas, err := m.ScanCodeModels(codeDirs)
	if err != nil {
		return nil, err
	}

	return CompareAll(canonicalSchemas, codeSchemas), nil
}

// NotifyIDE dispatches drift diagnostics to the connected JetBrains IDE (GoLand, RustRover, WebStorm, PyCharm).
func (m *ContractMonitor) NotifyIDE(ctx context.Context, report *DriftReport) error {
	if m.bridge == nil || report == nil || len(report.Violations) == 0 {
		return nil
	}

	client := m.bridge.GetClient()
	if client == nil {
		return nil
	}

	for _, v := range report.Violations {
		if v.TargetFile != "" {
			_ = m.bridge.OpenFile(ctx, v.TargetFile, v.Line, 1)
			break
		}
	}

	return nil
}
