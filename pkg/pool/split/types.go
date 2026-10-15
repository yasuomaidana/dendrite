package split

import (
	"errors"
	"time"

	"Dendrite/pkg/ui/modal"
)

// Standard typed errors for project splitter.
var (
	ErrSourceProjectNotFound = errors.New("source project directory does not exist")
	ErrModuleNotFound        = errors.New("specified module subpath does not exist in source project")
	ErrTargetDirRequired     = errors.New("target destination directory is required")
	ErrTargetDirNotEmpty     = errors.New("target destination directory already exists and is not empty")
	ErrSplitAborted          = errors.New("project split operation was aborted")
)

// PromptHandler handles interactive directory selection.
type PromptHandler func(m *modal.DirectoryPickerModal) (string, error)

// SplitRequest encapsulates parameters for extracting a module into a pool member.
type SplitRequest struct {
	SourceProjectDir string        `json:"source_project_dir"`
	ModuleSubpath    string        `json:"module_subpath"`
	NewMemberName    string        `json:"new_member_name"`
	TargetDirectory  string        `json:"target_directory,omitempty"`
	RegisterInPool   bool          `json:"register_in_pool"`
	PoolConfigFile   string        `json:"pool_config_file,omitempty"`
	WireLocalMode    bool          `json:"wire_local_mode"`
	Prompt           PromptHandler `json:"-"`
}

// SplitResult details the outcome of an extracted pool member project.
type SplitResult struct {
	SourceProject    string    `json:"source_project"`
	NewMemberName    string    `json:"new_member_name"`
	TargetDirectory  string    `json:"target_directory"`
	ExtractedFiles   []string  `json:"extracted_files"`
	ManifestCreated  string    `json:"manifest_created"`
	ParentWiredLocal bool      `json:"parent_wired_local"`
	PoolRegistered   bool      `json:"pool_registered"`
	Timestamp        time.Time `json:"timestamp"`
}
