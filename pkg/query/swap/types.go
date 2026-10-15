package swap

import (
	"errors"
	"time"
)

// SubsystemType denotes an application backend or I/O subsystem.
type SubsystemType string

const (
	SubsystemOpenGL  SubsystemType = "opengl"
	SubsystemDirectX SubsystemType = "directx"
	SubsystemWGPU    SubsystemType = "wgpu"
	SubsystemThreeJS SubsystemType = "threejs"
	SubsystemVulkan  SubsystemType = "vulkan"
)

// Standard typed errors.
var (
	ErrUnsupportedSubsystem       = errors.New("unsupported subsystem target")
	ErrDomainSimulationCorrupted   = errors.New("domain simulation kinematics parity check failed")
	ErrInvalidSourcesPath         = errors.New("invalid or empty sources path")
)

// SceneNode models a scene graph node with 3D kinematic transform state.
type SceneNode struct {
	ID          string      `json:"id"`
	Position    [3]float64  `json:"position"`
	Rotation    [4]float64  `json:"rotation"` // Quaternion: x, y, z, w
	Scale       [3]float64  `json:"scale"`
	WorldMatrix [16]float64 `json:"world_matrix"`
}

// SwapRequest defines parameters for replacing legacy subsystems with modern trait boundaries.
type SwapRequest struct {
	SourcesPath    string        `json:"sources_path"`
	FromSubsystem  SubsystemType `json:"from_subsystem"`
	ToSubsystem    SubsystemType `json:"to_subsystem"`
	TargetLanguage string        `json:"target_language"` // rust, go, typescript
	OutputDir      string        `json:"output_dir,omitempty"`
}

// SwapResult encapsulates the output and parity verification of a subsystem swap.
type SwapResult struct {
	FromSubsystem          SubsystemType `json:"from_subsystem"`
	ToSubsystem            SubsystemType `json:"to_subsystem"`
	TargetLanguage         string        `json:"target_language"`
	TraitBoundaryGenerated string        `json:"trait_boundary_generated"`
	PreservedDomainModules []string      `json:"preserved_domain_modules"`
	SynthesizedAdapters    []string      `json:"synthesized_adapters"`
	MatrixParityVerified   bool          `json:"matrix_parity_verified"`
	EmittedFiles           []string      `json:"emitted_files"`
	Timestamp              time.Time     `json:"timestamp"`
}
