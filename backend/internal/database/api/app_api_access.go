package api

import (
	"backend/internal/models"
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// AppAPIAccessAPI provides database operations for app API access configuration
type AppAPIAccessAPI struct{}

// GetAppAPIAccess returns API access configuration for a specific app
func (a *AppAPIAccessAPI) GetAppAPIAccess(ctx context.Context, appName string) (*models.AppAPIAccessResponse, error) {
	query := `
		SELECT app_name, api_access_enabled, allowed_operations, rate_limit_per_minute, created_at, updated_at
		FROM app_api_access 
		WHERE app_name = $1
	`
	
	var response models.AppAPIAccessResponse
	var allowedOpsArray pq.StringArray
	
	err := QueryRow(ctx, query, appName).Scan(
		&response.AppName, &response.APIAccessEnabled, &allowedOpsArray, 
		&response.RateLimitPerMinute, &response.CreatedAt, &response.UpdatedAt,
	)
	
	if err != nil {
		// Return default configuration if not found
		fmt.Printf("GetAppAPIAccess - No record found for app %s, returning defaults\n", appName)
		return &models.AppAPIAccessResponse{
			AppName:              appName,
			APIAccessEnabled:     false,
			AllowedOperations:    models.GetDefaultOperations(),
			RateLimitPerMinute:   60,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}, nil
	}
	
	response.AllowedOperations = []string(allowedOpsArray)
	fmt.Printf("GetAppAPIAccess - Found record for app %s: Enabled=%v\n", appName, response.APIAccessEnabled)
	return &response, nil
}

// SetAppAPIAccess creates or updates API access configuration for an app
func (a *AppAPIAccessAPI) SetAppAPIAccess(ctx context.Context, appName string, enabled bool, operations []string, rateLimit int) error {
	if operations == nil {
		operations = models.GetDefaultOperations()
	}
	if rateLimit <= 0 {
		rateLimit = 60
	}
	
	// Debug log
	fmt.Printf("SetAppAPIAccess DB - App: %s, Enabled: %v, Ops: %v, RateLimit: %d\n", appName, enabled, operations, rateLimit)
	
	query := `
		INSERT INTO app_api_access (app_name, api_access_enabled, allowed_operations, rate_limit_per_minute, created_at, updated_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (app_name) 
		DO UPDATE SET 
			api_access_enabled = $2,
			allowed_operations = $3,
			rate_limit_per_minute = $4,
			updated_at = CURRENT_TIMESTAMP
	`
	
	_, err := Exec(ctx, query, appName, enabled, pq.StringArray(operations), rateLimit)
	if err != nil {
		fmt.Printf("SetAppAPIAccess DB ERROR: %v\n", err)
		return fmt.Errorf("failed to set app API access: %w", err)
	}
	
	fmt.Printf("SetAppAPIAccess DB SUCCESS\n")
	return nil
}

// IsAppAPIAccessEnabled checks if an app has API access enabled (simple on/off)
func (a *AppAPIAccessAPI) IsAppAPIAccessEnabled(ctx context.Context, appName string) (bool, error) {
	query := `SELECT api_access_enabled FROM app_api_access WHERE app_name = $1`
	
	var enabled bool
	err := QueryRow(ctx, query, appName).Scan(&enabled)
	if err != nil {
		// No configuration found, default to disabled
		return false, nil
	}
	
	return enabled, nil
}

// CheckAppAPIAccess checks if an app has API access enabled (deprecated - keeping for compatibility)
func (a *AppAPIAccessAPI) CheckAppAPIAccess(ctx context.Context, appName string, operation string) (bool, error) {
	return a.IsAppAPIAccessEnabled(ctx, appName)
}

// Global instance
var AppAPIAccess = &AppAPIAccessAPI{}
