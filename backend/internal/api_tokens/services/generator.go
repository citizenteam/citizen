package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	// TokenPrefix is the prefix for all Citizen API tokens
	TokenPrefix = "ct_"
	// TokenLength is the total length of the token (excluding prefix) - 64 hex chars = 256 bits
	TokenLength = 64
	// PrefixDisplayLength is how many characters of the token to show for identification
	PrefixDisplayLength = 8
)

// GenerateAPIToken creates a new API token with the ct_ prefix
func GenerateAPIToken() (string, string, error) {
	// Generate random bytes
	bytes := make([]byte, TokenLength/2) // hex encoding doubles the length
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Convert to hex and add prefix
	tokenSuffix := hex.EncodeToString(bytes)
	fullToken := TokenPrefix + tokenSuffix

	// Create display prefix (first 8 chars after prefix)
	displayPrefix := TokenPrefix + tokenSuffix[:PrefixDisplayLength]

	return fullToken, displayPrefix, nil
}

// HashAPIToken creates a SHA-256 hash of the API token for storage
func HashAPIToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// ValidateAPITokenFormat checks if a token has the correct format
func ValidateAPITokenFormat(token string) bool {
	if !strings.HasPrefix(token, TokenPrefix) {
		return false
	}

	// Remove prefix and check length
	suffix := strings.TrimPrefix(token, TokenPrefix)
	return len(suffix) == TokenLength
}

// IsTokenExpired checks if a token is expired
func IsTokenExpired(expiresAt *time.Time) bool {
	if expiresAt == nil {
		return false // No expiration set
	}
	return time.Now().After(*expiresAt)
}

// ExtractAPIToken extracts API token from Authorization header only
// NOTE: Query parameter support removed for security - tokens in URLs can leak via logs, referer headers, browser history
func ExtractAPIToken(authHeader string) string {
	if authHeader == "" {
		return ""
	}

	// Support both "Bearer token" and raw token formats
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if ValidateAPITokenFormat(token) {
			return token
		}
	} else if ValidateAPITokenFormat(authHeader) {
		return authHeader
	}

	return ""
}
