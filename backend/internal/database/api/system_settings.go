package api

import (
	"context"
	"fmt"
	"strconv"
)

// SystemSetting represents a system configuration setting
type SystemSetting struct {
	ID          int    `json:"id"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	ValueType   string `json:"value_type"`
}

// SystemSettingsAPI handles system settings database operations
type SystemSettingsAPI struct{}

// GetSetting retrieves a single setting by key
func (api *SystemSettingsAPI) GetSetting(ctx context.Context, key string) (*SystemSetting, error) {
	if DB == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	query := `
		SELECT id, key, value, COALESCE(description, ''), COALESCE(value_type, 'string')
		FROM system_settings
		WHERE key = $1
	`

	setting := &SystemSetting{}
	err := DB.QueryRow(ctx, query, key).Scan(
		&setting.ID, &setting.Key, &setting.Value, &setting.Description, &setting.ValueType,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get setting %s: %w", key, err)
	}

	return setting, nil
}

// GetSettingValue retrieves the value of a setting as string
func (api *SystemSettingsAPI) GetSettingValue(ctx context.Context, key string, defaultValue string) string {
	setting, err := api.GetSetting(ctx, key)
	if err != nil {
		return defaultValue
	}
	return setting.Value
}

// GetSettingBool retrieves the value of a setting as bool
func (api *SystemSettingsAPI) GetSettingBool(ctx context.Context, key string, defaultValue bool) bool {
	setting, err := api.GetSetting(ctx, key)
	if err != nil {
		return defaultValue
	}
	return setting.Value == "true" || setting.Value == "1"
}

// GetSettingInt retrieves the value of a setting as int
func (api *SystemSettingsAPI) GetSettingInt(ctx context.Context, key string, defaultValue int) int {
	setting, err := api.GetSetting(ctx, key)
	if err != nil {
		return defaultValue
	}
	val, err := strconv.Atoi(setting.Value)
	if err != nil {
		return defaultValue
	}
	return val
}

// SetSetting updates or creates a setting
func (api *SystemSettingsAPI) SetSetting(ctx context.Context, key, value string) error {
	if DB == nil {
		return fmt.Errorf("database connection not initialized")
	}

	query := `
		INSERT INTO system_settings (key, value, updated_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (key) DO UPDATE SET
			value = $2,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := DB.Exec(ctx, query, key, value)
	if err != nil {
		return fmt.Errorf("failed to set setting %s: %w", key, err)
	}

	return nil
}

// GetAllSettings retrieves all system settings
func (api *SystemSettingsAPI) GetAllSettings(ctx context.Context) ([]SystemSetting, error) {
	if DB == nil {
		return nil, fmt.Errorf("database connection not initialized")
	}

	query := `
		SELECT id, key, value, COALESCE(description, ''), COALESCE(value_type, 'string')
		FROM system_settings
		ORDER BY key
	`

	rows, err := DB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get settings: %w", err)
	}
	defer rows.Close()

	var settings []SystemSetting
	for rows.Next() {
		var setting SystemSetting
		err := rows.Scan(&setting.ID, &setting.Key, &setting.Value, &setting.Description, &setting.ValueType)
		if err != nil {
			return nil, fmt.Errorf("failed to scan setting: %w", err)
		}
		settings = append(settings, setting)
	}

	return settings, nil
}

// GetBuildSettings retrieves all build-related settings as a map
func (api *SystemSettingsAPI) GetBuildSettings(ctx context.Context) (map[string]interface{}, error) {
	settings := map[string]interface{}{
		"deployment_queue_enabled": api.GetSettingBool(ctx, "deployment_queue_enabled", true),
		"deployment_queue_workers": api.GetSettingInt(ctx, "deployment_queue_workers", 3),
		"build_timeout_minutes":    api.GetSettingInt(ctx, "build_timeout_minutes", 30),
		"auto_cleanup_old_builds":  api.GetSettingBool(ctx, "auto_cleanup_old_builds", true),
		"max_builds_per_app":       api.GetSettingInt(ctx, "max_builds_per_app", 10),
	}
	return settings, nil
}

// UpdateBuildSettings updates multiple build settings at once
func (api *SystemSettingsAPI) UpdateBuildSettings(ctx context.Context, settings map[string]interface{}) error {
	for key, value := range settings {
		var strValue string
		switch v := value.(type) {
		case bool:
			strValue = strconv.FormatBool(v)
		case int:
			strValue = strconv.Itoa(v)
		case float64:
			strValue = strconv.Itoa(int(v))
		case string:
			strValue = v
		default:
			strValue = fmt.Sprintf("%v", v)
		}

		if err := api.SetSetting(ctx, key, strValue); err != nil {
			return err
		}
	}
	return nil
}

// Global instance
var SystemSettings = &SystemSettingsAPI{}
