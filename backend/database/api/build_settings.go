package api

import (
	"backend/models"
	"context"
	"encoding/json"
	"fmt"
)

// BuildSettingsAPI handles app build settings database operations
type BuildSettingsAPI struct{}

// BuildSettings is the global instance
var BuildSettings = &BuildSettingsAPI{}

// GetBuildSettings retrieves build settings for an app
func (b *BuildSettingsAPI) GetBuildSettings(ctx context.Context, appName string) (*models.AppBuildSettings, error) {
	query := `
		SELECT id, app_name, builder_type, dockerfile_path, build_env, created_at, updated_at
		FROM app_build_settings
		WHERE app_name = $1`

	var settings models.AppBuildSettings
	var buildEnvJSON []byte

	err := QueryRow(ctx, query, appName).Scan(
		&settings.ID,
		&settings.AppName,
		&settings.BuilderType,
		&settings.DockerfilePath,
		&buildEnvJSON,
		&settings.CreatedAt,
		&settings.UpdatedAt,
	)
	if err != nil {
		// Return default settings if not found
		return models.DefaultBuildSettings(appName), nil
	}

	// Parse build env JSON
	if len(buildEnvJSON) > 0 {
		if err := json.Unmarshal(buildEnvJSON, &settings.BuildEnv); err != nil {
			settings.BuildEnv = make(map[string]string)
		}
	}

	return &settings, nil
}

// SaveBuildSettings saves or updates build settings for an app
func (b *BuildSettingsAPI) SaveBuildSettings(ctx context.Context, settings *models.AppBuildSettings) error {
	if settings.AppName == "" {
		return fmt.Errorf("app name is required")
	}

	// Default values
	if settings.BuilderType == "" {
		settings.BuilderType = models.BuilderTypeAuto
	}
	if settings.DockerfilePath == "" {
		settings.DockerfilePath = "Dockerfile"
	}
	if settings.BuildEnv == nil {
		settings.BuildEnv = make(map[string]string)
	}

	buildEnvJSON, err := json.Marshal(settings.BuildEnv)
	if err != nil {
		return fmt.Errorf("failed to marshal build env: %w", err)
	}

	query := `
		INSERT INTO app_build_settings (app_name, builder_type, dockerfile_path, build_env, updated_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (app_name) DO UPDATE SET
			builder_type = EXCLUDED.builder_type,
			dockerfile_path = EXCLUDED.dockerfile_path,
			build_env = EXCLUDED.build_env,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id`

	return QueryRow(ctx, query,
		settings.AppName,
		settings.BuilderType,
		settings.DockerfilePath,
		buildEnvJSON,
	).Scan(&settings.ID)
}

// SetBuilderType updates only the builder type for an app
func (b *BuildSettingsAPI) SetBuilderType(ctx context.Context, appName string, builderType models.BuilderType) error {
	if appName == "" {
		return fmt.Errorf("app name is required")
	}

	// Validate builder type
	switch builderType {
	case models.BuilderTypeAuto, models.BuilderTypeDockerfile, models.BuilderTypeNixpacks:
		// Valid
	default:
		return fmt.Errorf("invalid builder type: %s", builderType)
	}

	query := `
		INSERT INTO app_build_settings (app_name, builder_type, updated_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (app_name) DO UPDATE SET
			builder_type = EXCLUDED.builder_type,
			updated_at = CURRENT_TIMESTAMP`

	_, err := Exec(ctx, query, appName, builderType)
	return err
}

// SetDockerfilePath updates the dockerfile path for an app
func (b *BuildSettingsAPI) SetDockerfilePath(ctx context.Context, appName, path string) error {
	if appName == "" {
		return fmt.Errorf("app name is required")
	}
	if path == "" {
		path = "Dockerfile"
	}

	query := `
		INSERT INTO app_build_settings (app_name, dockerfile_path, updated_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (app_name) DO UPDATE SET
			dockerfile_path = EXCLUDED.dockerfile_path,
			updated_at = CURRENT_TIMESTAMP`

	_, err := Exec(ctx, query, appName, path)
	return err
}

// DeleteBuildSettings removes build settings for an app
func (b *BuildSettingsAPI) DeleteBuildSettings(ctx context.Context, appName string) error {
	query := `DELETE FROM app_build_settings WHERE app_name = $1`
	_, err := Exec(ctx, query, appName)
	return err
}

// GetBuilderType returns the builder type for an app (convenience method)
func (b *BuildSettingsAPI) GetBuilderType(ctx context.Context, appName string) (string, error) {
	settings, err := b.GetBuildSettings(ctx, appName)
	if err != nil {
		return "nixpacks", err
	}
	return settings.ResolvedBuilderType(), nil
}

