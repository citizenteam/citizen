package api

import (
	apitokenservices "backend/api_tokens/services"
	"backend/models"
	"context"
	"fmt"
	"time"
)

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
func (a *APITokensAPI) ValidateAPIToken(ctx context.Context, rawToken string) (*models.User, error) {
	// Validate format
	if !apitokenservices.ValidateAPITokenFormat(rawToken) {
		return nil, fmt.Errorf("invalid token format")
	}

	// Hash token
	tokenHash := apitokenservices.HashAPIToken(rawToken)

	// Find token and user info
	query := `
		SELECT u.id, u.username, u.email, u.created_at, u.updated_at, t.expires_at
		FROM api_tokens t
		JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = $1 AND t.is_active = true
	`

	var user models.User
	var expiresAt *time.Time
	err := QueryRow(ctx, query, tokenHash).Scan(
		&user.ID, &user.Username, &user.Email, &user.CreatedAt, &user.UpdatedAt, &expiresAt,
	)

	if err != nil {
		return nil, fmt.Errorf("token not found or inactive: %w", err)
	}

	// Check expiration
	if apitokenservices.IsTokenExpired(expiresAt) {
		return nil, fmt.Errorf("token expired")
	}

	return &user, nil
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

// DeleteAPIToken deletes an API token
func (a *APITokensAPI) DeleteAPIToken(ctx context.Context, userID int, tokenID int) error {
	query := `DELETE FROM api_tokens WHERE id = $1 AND user_id = $2`

	result, err := Exec(ctx, query, tokenID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete API token: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("API token not found")
	}

	return nil
}

// Global instance
var APITokens = &APITokensAPI{}
