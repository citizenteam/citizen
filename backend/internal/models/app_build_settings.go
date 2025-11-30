package models

import (
	"time"
)

// BuilderType represents the type of builder to use
type BuilderType string

const (
	BuilderTypeAuto       BuilderType = "auto"       // Auto-detect, defaults to nixpacks
	BuilderTypeDockerfile BuilderType = "dockerfile" // Use Dockerfile
	BuilderTypeNixpacks   BuilderType = "nixpacks"   // Use Nixpacks explicitly
)

// AppBuildSettings represents build configuration for an app
type AppBuildSettings struct {
	ID             int         `json:"id"`
	AppName        string      `json:"app_name"`
	BuilderType    BuilderType `json:"builder_type"`
	DockerfilePath string      `json:"dockerfile_path"`
	BuildEnv       map[string]string `json:"build_env,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// DefaultBuildSettings returns default build settings for a new app
func DefaultBuildSettings(appName string) *AppBuildSettings {
	return &AppBuildSettings{
		AppName:        appName,
		BuilderType:    BuilderTypeAuto,
		DockerfilePath: "Dockerfile",
		BuildEnv:       make(map[string]string),
	}
}

// IsDockerfile returns true if builder type is dockerfile
func (s *AppBuildSettings) IsDockerfile() bool {
	return s.BuilderType == BuilderTypeDockerfile
}

// IsNixpacks returns true if builder type is nixpacks or auto
func (s *AppBuildSettings) IsNixpacks() bool {
	return s.BuilderType == BuilderTypeNixpacks || s.BuilderType == BuilderTypeAuto
}

// ResolvedBuilderType returns the actual builder to use
// For "auto", it returns "nixpacks" as the default
func (s *AppBuildSettings) ResolvedBuilderType() string {
	switch s.BuilderType {
	case BuilderTypeDockerfile:
		return "dockerfile"
	case BuilderTypeNixpacks, BuilderTypeAuto:
		return "nixpacks"
	default:
		return "nixpacks"
	}
}

