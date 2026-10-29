package bootstrap

import (
	"time"
)

// LanguageStack identifies the programming language or ecosystem detected in a repository.
type LanguageStack string

const (
	StackGo         LanguageStack = "go"
	StackRust       LanguageStack = "rust"
	StackTypeScript LanguageStack = "typescript"
	StackJavaScript LanguageStack = "javascript"
	StackPython     LanguageStack = "python"
	StackGeneric    LanguageStack = "generic"
)

// ToolchainInfo captures metadata, availability, and commands for a project toolchain.
type ToolchainInfo struct {
	Name           string `json:"name"`
	Binary         string `json:"binary"`
	Installed      bool   `json:"installed"`
	Version        string `json:"version,omitempty"`
	ManifestFile   string `json:"manifest_file,omitempty"`
	LockFile       string `json:"lock_file,omitempty"`
	TestCommand    string `json:"test_command"`
	BuildCommand   string `json:"build_command"`
	InstallCommand string `json:"install_command,omitempty"`
}

// ProjectFingerprint captures repository architecture, languages, and toolchains.
type ProjectFingerprint struct {
	RootPath         string                   `json:"root_path"`
	Name             string                   `json:"name"`
	PrimaryStack     LanguageStack            `json:"primary_stack"`
	SecondaryStacks  []LanguageStack          `json:"secondary_stacks,omitempty"`
	Toolchains       []string                 `json:"toolchains,omitempty"`
	ToolchainDetails map[string]ToolchainInfo `json:"toolchain_details,omitempty"`
	HasGoMod         bool                     `json:"has_go_mod"`
	HasCargoToml     bool                     `json:"has_cargo_toml"`
	HasPackageJSON   bool                     `json:"has_package_json"`
	HasPyProject     bool                     `json:"has_pyproject"`
	HasCargo         bool                     `json:"has_cargo"`
	HasBun           bool                     `json:"has_bun"`
	HasUV            bool                     `json:"has_uv"`
	GoVersion        string                   `json:"go_version,omitempty"`
	Linters          []string                 `json:"linters,omitempty"`
	TestFrameworks   []string                 `json:"test_frameworks,omitempty"`
}

// CICDStatus represents the configuration state and toolchain availability of CI/CD.
type CICDStatus struct {
	DefaultTool         string          `json:"default_tool"`
	ToolInstalled       bool            `json:"tool_installed"`
	ToolBinary          string          `json:"tool_binary"`
	ToolInstallCommand  string          `json:"tool_install_command,omitempty"`
	CopilotInstructions string          `json:"copilot_instructions_path"`
	CIWorkflowPath      string          `json:"ci_workflow_path"`
	CoverageThreshold   float64         `json:"coverage_threshold"`
	Toolchains          []ToolchainInfo `json:"toolchains,omitempty"`
	FilesCreated        []string        `json:"files_created,omitempty"`
	FilesUpdated        []string        `json:"files_updated,omitempty"`
	Enforced            bool            `json:"enforced"`
}

// PrepareOptions configures the environment preparation and synchronization pipeline.
type PrepareOptions struct {
	ProjectPath string `json:"project_path"`
	DefaultTool string `json:"default_tool"` // e.g. "copilot"
	EnforceCICD bool   `json:"enforce_cicd"` // default true
	SyncSkills  bool   `json:"sync_skills"`  // default true
	DryRun      bool   `json:"dry_run"`
}

// PrepareReport summarizes the outcome of environment preparation, agent synchronization, and CI/CD enforcement.
type PrepareReport struct {
	ProjectPath  string             `json:"project_path"`
	Stack        ProjectFingerprint `json:"stack"`
	FilesCreated []string           `json:"files_created"`
	FilesUpdated []string           `json:"files_updated"`
	CICD         CICDStatus         `json:"cicd"`
	SkillsSynced []string           `json:"skills_synced,omitempty"`
	// Notes say what Prepare deliberately left alone, for example a hand-written CI workflow.
	Notes    []string      `json:"notes,omitempty"`
	Duration time.Duration `json:"duration"`
	Success  bool          `json:"success"`
}
