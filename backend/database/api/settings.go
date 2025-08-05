package api

import (
	"context"
	"fmt"

	"backend/models"
)

// SettingsAPI provides settings-related database operations

// CreateAppPublicSetting creates a new app public setting
func (s *SettingsAPI) CreateAppPublicSetting(ctx context.Context, setting *models.AppPublicSetting) error {
	if err := ValidateArgs(setting.AppName); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		INSERT INTO app_public_settings (app_name, is_public, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id`

	now := GetCurrentTimestamp()
	err := QueryRow(ctx, query, setting.AppName, setting.IsPublic, now, now).Scan(&setting.ID)
	if err != nil {
		return fmt.Errorf("failed to create app public setting: %w", err)
	}

	return nil
}

// GetAppPublicSetting retrieves app public setting by app name
func (s *SettingsAPI) GetAppPublicSetting(ctx context.Context, appName string) (*models.AppPublicSetting, error) {
	if err := ValidateArgs(appName); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT id, app_name, is_public, created_at, updated_at
		FROM app_public_settings 
		WHERE app_name = $1`

	setting := &models.AppPublicSetting{}
	err := QueryRow(ctx, query, appName).Scan(
		&setting.ID, &setting.AppName, &setting.IsPublic,
		&setting.CreatedAt, &setting.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get app public setting: %w", err)
	}

	return setting, nil
}

// UpdateAppPublicSetting updates an app public setting
func (s *SettingsAPI) UpdateAppPublicSetting(ctx context.Context, appName string, isPublic bool) error {
	if err := ValidateArgs(appName, isPublic); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `UPDATE app_public_settings SET is_public = $2, updated_at = $3 WHERE app_name = $1`
	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, appName, isPublic, now)
	if err != nil {
		return fmt.Errorf("failed to update app public setting: %w", err)
	}

	return nil
}

// UpsertAppPublicSetting creates or updates an app public setting
func (s *SettingsAPI) UpsertAppPublicSetting(ctx context.Context, appName string, isPublic bool) error {
	if err := ValidateArgs(appName, isPublic); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Check if setting exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM app_public_settings WHERE app_name = $1)`
	err := QueryRow(ctx, checkQuery, appName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check existing setting: %w", err)
	}

	now := GetCurrentTimestamp()
	if exists {
		// Update existing
		query := `UPDATE app_public_settings SET is_public = $2, updated_at = $3 WHERE app_name = $1`
		_, err := Exec(ctx, query, appName, isPublic, now)
		if err != nil {
			return fmt.Errorf("failed to update app public setting: %w", err)
		}
	} else {
		// Create new
		query := `INSERT INTO app_public_settings (app_name, is_public, created_at, updated_at) VALUES ($1, $2, $3, $4)`
		_, err := Exec(ctx, query, appName, isPublic, now, now)
		if err != nil {
			return fmt.Errorf("failed to create app public setting: %w", err)
		}
	}

	return nil
}

// IsAppPublic checks if an app is public
func (s *SettingsAPI) IsAppPublic(ctx context.Context, appName string) (bool, error) {
	if err := ValidateArgs(appName); err != nil {
		return false, fmt.Errorf("validation failed: %w", err)
	}

	query := `SELECT COALESCE(is_public, false) FROM app_public_settings WHERE app_name = $1`
	var isPublic bool
	err := QueryRow(ctx, query, appName).Scan(&isPublic)
	if err != nil {
		// If no setting exists, default to false (not public)
		return false, nil
	}

	return isPublic, nil
}

// ListPublicApps retrieves all public apps
func (s *SettingsAPI) ListPublicApps(ctx context.Context) ([]string, error) {
	query := `SELECT app_name FROM app_public_settings WHERE is_public = true`
	rows, err := Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list public apps: %w", err)
	}
	defer rows.Close()

	var appNames []string
	for rows.Next() {
		var appName string
		err := rows.Scan(&appName)
		if err != nil {
			return nil, fmt.Errorf("failed to scan app name: %w", err)
		}
		appNames = append(appNames, appName)
	}

	return appNames, nil
}

// DeleteAppPublicSetting deletes an app public setting
func (s *SettingsAPI) DeleteAppPublicSetting(ctx context.Context, appName string) error {
	if err := ValidateArgs(appName); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `DELETE FROM app_public_settings WHERE app_name = $1`
	_, err := Exec(ctx, query, appName)
	if err != nil {
		return fmt.Errorf("failed to delete app public setting: %w", err)
	}

	return nil
}

// App Custom Domains Management

// CreateCustomDomain creates a new custom domain for an app
func (s *SettingsAPI) CreateCustomDomain(ctx context.Context, appName, domain string) error {
	if err := ValidateArgs(appName, domain); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		INSERT INTO app_custom_domains (app_name, domain, is_active, created_at, updated_at)
		VALUES ($1, $2, true, $3, $4)`

	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, appName, domain, now, now)
	if err != nil {
		return fmt.Errorf("failed to create custom domain: %w", err)
	}

	return nil
}

// GetCustomDomains retrieves all custom domains for an app
func (s *SettingsAPI) GetCustomDomains(ctx context.Context, appName string) ([]string, error) {
	if err := ValidateArgs(appName); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `SELECT domain FROM app_custom_domains WHERE app_name = $1 AND is_active = true ORDER BY created_at`
	rows, err := Query(ctx, query, appName)
	if err != nil {
		return nil, fmt.Errorf("failed to get custom domains: %w", err)
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var domain string
		err := rows.Scan(&domain)
		if err != nil {
			return nil, fmt.Errorf("failed to scan domain: %w", err)
		}
		domains = append(domains, domain)
	}

	return domains, nil
}

// DeleteCustomDomain deletes a custom domain for an app
func (s *SettingsAPI) DeleteCustomDomain(ctx context.Context, appName, domain string) error {
	if err := ValidateArgs(appName, domain); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `DELETE FROM app_custom_domains WHERE app_name = $1 AND domain = $2`
	_, err := Exec(ctx, query, appName, domain)
	if err != nil {
		return fmt.Errorf("failed to delete custom domain: %w", err)
	}

	return nil
}

// ActivateCustomDomain activates a custom domain
func (s *SettingsAPI) ActivateCustomDomain(ctx context.Context, appName, domain string) error {
	if err := ValidateArgs(appName, domain); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `UPDATE app_custom_domains SET is_active = true, updated_at = $3 WHERE app_name = $1 AND domain = $2`
	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, appName, domain, now)
	if err != nil {
		return fmt.Errorf("failed to activate custom domain: %w", err)
	}

	return nil
}

// DeactivateCustomDomain deactivates a custom domain
func (s *SettingsAPI) DeactivateCustomDomain(ctx context.Context, appName, domain string) error {
	if err := ValidateArgs(appName, domain); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `UPDATE app_custom_domains SET is_active = false, updated_at = $3 WHERE app_name = $1 AND domain = $2`
	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, appName, domain, now)
	if err != nil {
		return fmt.Errorf("failed to deactivate custom domain: %w", err)
	}

	return nil
}

// CustomDomainExists checks if a custom domain exists
func (s *SettingsAPI) CustomDomainExists(ctx context.Context, domain string) (bool, error) {
	if err := ValidateArgs(domain); err != nil {
		return false, fmt.Errorf("validation failed: %w", err)
	}

	query := `SELECT EXISTS(SELECT 1 FROM app_custom_domains WHERE domain = $1 AND is_active = true)`
	var exists bool
	err := QueryRow(ctx, query, domain).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check domain existence: %w", err)
	}

	return exists, nil
}

// GetAppByCustomDomain retrieves app name by custom domain
func (s *SettingsAPI) GetAppByCustomDomain(ctx context.Context, domain string) (string, error) {
	if err := ValidateArgs(domain); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	query := `SELECT app_name FROM app_custom_domains WHERE domain = $1 AND is_active = true`
	var appName string
	err := QueryRow(ctx, query, domain).Scan(&appName)
	if err != nil {
		return "", fmt.Errorf("failed to get app by domain: %w", err)
	}

	return appName, nil
}

// GetAllActiveCustomDomains retrieves all active custom domains
func (s *SettingsAPI) GetAllActiveCustomDomains(ctx context.Context) ([]models.AppCustomDomain, error) {
	query := `SELECT id, app_name, domain, is_active, created_at, updated_at FROM app_custom_domains WHERE is_active = true ORDER BY created_at DESC`
	rows, err := Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active custom domains: %w", err)
	}
	defer rows.Close()

	var domains []models.AppCustomDomain
	for rows.Next() {
		var domain models.AppCustomDomain
		err := rows.Scan(&domain.ID, &domain.AppName, &domain.Domain, &domain.IsActive, &domain.CreatedAt, &domain.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan custom domain: %w", err)
		}
		domains = append(domains, domain)
	}

	return domains, nil
} 