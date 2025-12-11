package api

import (
	"crypto/subtle"

	apitokenservices "backend/internal/api_tokens/services"
	"backend/internal/models"
	"backend/pkg/logger"
	"context"
	"fmt"
	"time"
)

var apiTokenLog = logger.Default().WithComponent("api-tokens")

// APITokensAPI provides database operations for API tokens
type APITokensAPI struct{}

// CreateAPIToken creates a new API token for a user
func (a *APITokensAPI) CreateAPIToken(ctx context.Context, userID int, req *models.APITokenRequest) (*models.APITokenResponse, error) {
	// Generate new token
	rawToken, tokenPrefix, err := apitokenservices.GenerateAPIToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Hash token for storage
	tokenHash := apitokenservices.HashAPIToken(rawToken)

	// Insert token into database
	query := `
		INSERT INTO api_tokens (user_id, token_hash, token_prefix, name, description, expires_at, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id, created_at
	`

	var tokenID int
	var createdAt time.Time
	err = QueryRow(ctx, query, userID, tokenHash, tokenPrefix, req.Name, req.Description, req.ExpiresAt).
		Scan(&tokenID, &createdAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create API token: %w", err)
	}

	// Return response with raw token (only time it's returned)
	return &models.APITokenResponse{
		ID:          uint(tokenID),
		TokenPrefix: tokenPrefix,
		Name:        req.Name,
		Description: req.Description,
		ExpiresAt:   req.ExpiresAt,
		CreatedAt:   createdAt,
		Token:       rawToken, // Raw token included only in creation response
	}, nil
}

// ListAPITokens returns all API tokens for a user (without raw tokens)
func (a *APITokensAPI) ListAPITokens(ctx context.Context, userID int) ([]*models.APITokenListResponse, error) {
	query := `
		SELECT id, token_prefix, name, description, expires_at, last_used_at, usage_count, is_active, created_at, updated_at
		FROM api_tokens 
		WHERE user_id = $1 
		ORDER BY created_at DESC
	`

	rows, err := Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API tokens: %w", err)
	}
	defer rows.Close()

	var response []*models.APITokenListResponse
	for rows.Next() {
		var token models.APITokenListResponse
		err := rows.Scan(
			&token.ID, &token.TokenPrefix, &token.Name, &token.Description,
			&token.ExpiresAt, &token.LastUsedAt, &token.UsageCount, &token.IsActive,
			&token.CreatedAt, &token.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API token: %w", err)
		}
		response = append(response, &token)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return response, nil
}

// ValidateAPIToken validates a raw token and returns the associated user info
// Uses constant-time comparison to prevent timing attacks
// Logs all attempts (success/fail) to audit table
func (a *APITokensAPI) ValidateAPIToken(ctx context.Context, rawToken, clientIP, userAgent, appName, citizenauthUserID, orgID string) (*models.User, error) {
	// Validate format first (fast rejection for malformed tokens)
	if !apitokenservices.ValidateAPITokenFormat(rawToken) {
		// Log failed attempt - invalid format
		go a.LogTokenAuditEvent(ctx, "", nil, citizenauthUserID, orgID, "failed", "invalid_format", clientIP, userAgent, appName)
		return nil, fmt.Errorf("invalid token format")
	}

	// Hash the provided token
	providedHash := apitokenservices.HashAPIToken(rawToken)

	// Get token prefix for logging (safe to log)
	tokenPrefix := rawToken[:11] // "ct_" + first 8 chars

	// Fetch all active tokens with their hashes for constant-time comparison
	// This prevents timing attacks based on database query time
	query := `
		SELECT t.token_hash, t.expires_at, u.id, u.username, u.email, u.created_at, u.updated_at
		FROM api_tokens t
		JOIN users u ON u.id = t.user_id
		WHERE t.is_active = true
	`

	rows, err := Query(ctx, query)
	if err != nil {
		apiTokenLog.WithFields(map[string]interface{}{
			"ip":     clientIP,
			"prefix": tokenPrefix,
			"error":  err.Error(),
		}).Error("API token validation: database error")
		return nil, fmt.Errorf("database error")
	}
	defer rows.Close()

	var matchedUser *models.User
	var matchedExpiresAt *time.Time

	for rows.Next() {
		var storedHash string
		var expiresAt *time.Time
		var user models.User

		if err := rows.Scan(&storedHash, &expiresAt, &user.ID, &user.Username, &user.Email, &user.CreatedAt, &user.UpdatedAt); err != nil {
			continue
		}

		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(providedHash), []byte(storedHash)) == 1 {
			matchedUser = &user
			matchedExpiresAt = expiresAt
			// Don't break - continue iterating to maintain constant time
		}
	}

	if matchedUser == nil {
		// Log failed attempt - token not found
		go a.LogTokenAuditEvent(ctx, tokenPrefix, nil, citizenauthUserID, orgID, "failed", "not_found", clientIP, userAgent, appName)
		return nil, fmt.Errorf("token not found or inactive")
	}

	// Convert uint to int for logging
	localUserID := int(matchedUser.ID)

	// Check expiration
	if apitokenservices.IsTokenExpired(matchedExpiresAt) {
		// Log failed attempt - token expired
		go a.LogTokenAuditEvent(ctx, tokenPrefix, &localUserID, citizenauthUserID, orgID, "failed", "expired", clientIP, userAgent, appName)
		return nil, fmt.Errorf("token expired")
	}

	// Log successful validation
	go a.LogTokenAuditEvent(ctx, tokenPrefix, &localUserID, citizenauthUserID, orgID, "success", "", clientIP, userAgent, appName)

	return matchedUser, nil
}

// UpdateTokenUsage updates token usage statistics
func (a *APITokensAPI) UpdateTokenUsage(ctx context.Context, rawToken string, ipAddress string) error {
	tokenHash := apitokenservices.HashAPIToken(rawToken)

	query := `
		UPDATE api_tokens 
		SET last_used_at = CURRENT_TIMESTAMP, 
		    usage_count = usage_count + 1,
		    last_used_ip = $2,
		    updated_at = CURRENT_TIMESTAMP
		WHERE token_hash = $1 AND is_active = true
	`

	_, err := Exec(ctx, query, tokenHash, ipAddress)
	if err != nil {
		return fmt.Errorf("failed to update token usage: %w", err)
	}

	return nil
}

// DeleteAPIToken soft-deletes an API token with audit trail
func (a *APITokensAPI) DeleteAPIToken(ctx context.Context, userID int, tokenID int) error {
	// Soft delete with audit trail
	query := `
		UPDATE api_tokens 
		SET is_active = false, 
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $2 AND is_active = true
	`

	result, err := Exec(ctx, query, tokenID, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke API token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("API token not found or already revoked")
	}

	apiTokenLog.WithFields(map[string]interface{}{
		"user_id":  userID,
		"token_id": tokenID,
	}).Info("API token revoked")

	return nil
}

// LogTokenAuditEvent logs an API token audit event to database
func (a *APITokensAPI) LogTokenAuditEvent(ctx context.Context, tokenPrefix string, localUserID *int, citizenauthUserID, orgID, eventType, failureReason, ipAddress, userAgent, appName string) error {
	query := `
		INSERT INTO api_token_audit_logs (token_prefix, local_user_id, citizenauth_user_id, organization_id, event_type, failure_reason, ip_address, user_agent, app_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := Exec(ctx, query, tokenPrefix, localUserID, nilIfEmpty(citizenauthUserID), nilIfEmpty(orgID), eventType, nilIfEmpty(failureReason), ipAddress, nilIfEmpty(userAgent), nilIfEmpty(appName))
	if err != nil {
		apiTokenLog.WithField("error", err.Error()).Error("Failed to log audit event")
		return err
	}

	return nil
}

// nilIfEmpty returns nil if string is empty, otherwise returns pointer to string
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// APITokenAuditLog represents an audit log entry
type APITokenAuditLog struct {
	ID            int       `json:"id"`
	TokenPrefix   *string   `json:"token_prefix,omitempty"`
	UserID        *int      `json:"user_id,omitempty"`
	Username      *string   `json:"username,omitempty"`
	EventType     string    `json:"event_type"`
	FailureReason *string   `json:"failure_reason,omitempty"`
	IPAddress     string    `json:"ip_address"`
	UserAgent     *string   `json:"user_agent,omitempty"`
	AppName       *string   `json:"app_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// GetTokenAuditLogs returns audit logs filtered by RLS (with pagination)
func (a *APITokensAPI) GetTokenAuditLogs(ctx context.Context, eventType string, limit, offset int) ([]*APITokenAuditLog, int64, error) {
	// Count total (RLS applied)
	countQuery := `SELECT COUNT(*) FROM api_token_audit_logs WHERE ($1 = '' OR event_type = $1)`
	var total int64
	if err := QueryRow(ctx, countQuery, eventType).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	// Get logs with user info (RLS applied)
	query := `
		SELECT l.id, l.token_prefix, l.local_user_id, u.username, l.event_type, l.failure_reason, 
		       l.ip_address, l.user_agent, l.app_name, l.created_at
		FROM api_token_audit_logs l
		LEFT JOIN users u ON u.id = l.local_user_id
		WHERE ($1 = '' OR l.event_type = $1)
		ORDER BY l.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := Query(ctx, query, eventType, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*APITokenAuditLog
	for rows.Next() {
		var log APITokenAuditLog
		if err := rows.Scan(
			&log.ID, &log.TokenPrefix, &log.UserID, &log.Username, &log.EventType,
			&log.FailureReason, &log.IPAddress, &log.UserAgent, &log.AppName, &log.CreatedAt,
		); err != nil {
			continue
		}
		logs = append(logs, &log)
	}

	return logs, total, nil
}

// GetAllTokenAuditLogs returns ALL audit logs (for super admins - bypasses RLS)
func (a *APITokensAPI) GetAllTokenAuditLogs(ctx context.Context, eventType string, limit, offset int) ([]*APITokenAuditLog, int64, error) {
	// Count total (no RLS filter)
	countQuery := `SELECT COUNT(*) FROM api_token_audit_logs WHERE ($1 = '' OR event_type = $1)`
	var total int64
	if err := QueryRow(ctx, countQuery, eventType).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	// Get all logs with user info (no RLS filter)
	query := `
		SELECT l.id, l.token_prefix, l.local_user_id, u.username, l.event_type, l.failure_reason, 
		       l.ip_address, l.user_agent, l.app_name, l.created_at
		FROM api_token_audit_logs l
		LEFT JOIN users u ON u.id = l.local_user_id
		WHERE ($1 = '' OR l.event_type = $1)
		ORDER BY l.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := Query(ctx, query, eventType, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*APITokenAuditLog
	for rows.Next() {
		var log APITokenAuditLog
		if err := rows.Scan(
			&log.ID, &log.TokenPrefix, &log.UserID, &log.Username, &log.EventType,
			&log.FailureReason, &log.IPAddress, &log.UserAgent, &log.AppName, &log.CreatedAt,
		); err != nil {
			continue
		}
		logs = append(logs, &log)
	}

	return logs, total, nil
}

// GetRecentFailedAttempts returns recent failed login attempts grouped by IP for an organization
func (a *APITokensAPI) GetRecentFailedAttempts(ctx context.Context, orgID string, hours int, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT ip_address, COUNT(*) as attempt_count, 
		       MAX(created_at) as last_attempt,
		       array_agg(DISTINCT failure_reason) as reasons
		FROM api_token_audit_logs
		WHERE event_type = 'failed' 
		  AND ($1 = '' OR organization_id = $1::uuid)
		  AND created_at > NOW() - INTERVAL '1 hour' * $2
		GROUP BY ip_address
		ORDER BY attempt_count DESC
		LIMIT $3
	`

	rows, err := Query(ctx, query, orgID, hours, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get failed attempts: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ipAddress string
		var attemptCount int
		var lastAttempt time.Time
		var reasons []string

		if err := rows.Scan(&ipAddress, &attemptCount, &lastAttempt, &reasons); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"ip_address":    ipAddress,
			"attempt_count": attemptCount,
			"last_attempt":  lastAttempt,
			"reasons":       reasons,
		})
	}

	return results, nil
}

// GetTokenUsageStats returns usage statistics for a specific token
func (a *APITokensAPI) GetTokenUsageStats(ctx context.Context, tokenID int, userID int) (map[string]interface{}, error) {
	// Get token info
	tokenQuery := `
		SELECT token_prefix, last_used_at, last_used_ip, usage_count
		FROM api_tokens
		WHERE id = $1 AND user_id = $2
	`

	var tokenPrefix string
	var lastUsedAt *time.Time
	var lastUsedIP *string
	var usageCount int

	if err := QueryRow(ctx, tokenQuery, tokenID, userID).Scan(&tokenPrefix, &lastUsedAt, &lastUsedIP, &usageCount); err != nil {
		return nil, fmt.Errorf("token not found: %w", err)
	}

	// Get recent activity from audit logs
	activityQuery := `
		SELECT event_type, ip_address, created_at
		FROM api_token_audit_logs
		WHERE token_prefix = $1
		ORDER BY created_at DESC
		LIMIT 10
	`

	rows, err := Query(ctx, activityQuery, tokenPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to get token activity: %w", err)
	}
	defer rows.Close()

	var recentActivity []map[string]interface{}
	for rows.Next() {
		var eventType, ipAddress string
		var createdAt time.Time
		if err := rows.Scan(&eventType, &ipAddress, &createdAt); err != nil {
			continue
		}
		recentActivity = append(recentActivity, map[string]interface{}{
			"event_type": eventType,
			"ip_address": ipAddress,
			"timestamp":  createdAt,
		})
	}

	return map[string]interface{}{
		"token_prefix":    tokenPrefix,
		"last_used_at":    lastUsedAt,
		"last_used_ip":    lastUsedIP,
		"usage_count":     usageCount,
		"recent_activity": recentActivity,
	}, nil
}

// Global instance
var APITokens = &APITokensAPI{}
