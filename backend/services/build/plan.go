package build

import (
	"time"
)

// BuildPlan represents a complete build configuration
type BuildPlan struct {
	AppName      string
	BuildType    string            // "nixpacks" | "dockerfile" | "compose"
	Builder      string            // Builder tool name
	GitURL       string
	GitBranch    string
	ImageRef     string            // Target image reference
	BuildCommand string            // Command to execute
	EnvVars      map[string]string // Build-time environment variables
	Registry     RegistryConfig
	Metadata     map[string]string // Additional metadata
	CreatedAt    time.Time
}

// JobConfig holds configuration for a Kubernetes build job
type JobConfig struct {
	JobName      string
	AppName      string
	GitURL       string
	GitBranch    string
	BuildCommand string
	ImageRef     string
	Registry     RegistryConfig
	EnvVars      map[string]string
	Resources    ResourceRequirements
}

// ResourceRequirements defines resource limits for build jobs
type ResourceRequirements struct {
	CPURequest    string // e.g. "500m"
	CPULimit      string // e.g. "2000m"
	MemoryRequest string // e.g. "512Mi"
	MemoryLimit   string // e.g. "2Gi"
}

// DefaultResources returns default resource requirements for build jobs
func DefaultResources() ResourceRequirements {
	return ResourceRequirements{
		CPURequest:    "500m",
		CPULimit:      "2000m",
		MemoryRequest: "512Mi",
		MemoryLimit:   "2Gi",
	}
}

// ValidatePlan checks if a build plan is valid
func ValidatePlan(plan *BuildPlan) error {
	if plan.AppName == "" {
		return ErrMissingAppName
	}
	if plan.GitURL == "" {
		return ErrMissingGitURL
	}
	if plan.BuildType == "" {
		return ErrMissingBuildType
	}
	return nil
}

// Build plan errors
var (
	ErrMissingAppName   = &BuildError{Message: "app name is required"}
	ErrMissingGitURL    = &BuildError{Message: "git URL is required"}
	ErrMissingBuildType = &BuildError{Message: "build type is required"}
)

// BuildError represents a build-related error
type BuildError struct {
	Message string
	Code    string
}

func (e *BuildError) Error() string {
	return e.Message
}

