package services

import (
	"backend/auth/models"
	"backend/database"
	"backend/utils"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Generate secure random ID
func GenerateSecureID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.URLEncoding.EncodeToString(b)
}

// CreateOrUpdateSSOSession creates or updates an SSO session (Redis-only storage)
func CreateOrUpdateSSOSession(userID int, mainDomain string, deviceID string, organizationID *string) string {
	sessionID := GenerateSecureID()

	session := &models.SSOSession{
		SessionID:      sessionID,
		UserID:         userID,
		MainDomain:     mainDomain,
		DeviceID:       deviceID,
		OrganizationID: organizationID,
		CreatedAt:      time.Now(),
		LastActivity:   time.Now(),
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}

	// Store in Redis only (cluster-safe)
	if data, err := json.Marshal(session); err == nil {
		if err := database.SetWithTTL("sso_session:"+sessionID, string(data), 24*time.Hour); err != nil {
			utils.ErrorLog("Failed to store SSO session in Redis: %v", err)
		}
	}

	return sessionID
}

// GetSSOSession retrieves an SSO session by ID from Redis
func GetSSOSession(sessionID string) (*models.SSOSession, error) {
	utils.SessionDebugLog(sessionID, "GetSSOSession called")

	data, err := database.Get("sso_session:" + sessionID)
	if err != nil || data == "" {
		utils.SessionDebugLog(sessionID, "Session not found in Redis: %v", err)
		return nil, fmt.Errorf("session not found")
	}

	utils.SessionDebugLog(sessionID, "Found session in Redis")
	var session models.SSOSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		utils.SessionDebugLog(sessionID, "Failed to unmarshal Redis data: %v", err)
		return nil, fmt.Errorf("invalid session data")
	}

	if time.Now().After(session.ExpiresAt) {
		utils.SessionDebugLog(sessionID, "Session expired in Redis. ExpiresAt: %v, Now: %v", session.ExpiresAt, time.Now())
		// Clean up expired session
		database.Delete("sso_session:" + sessionID)
		return nil, fmt.Errorf("session expired")
	}

	utils.SessionDebugLog(sessionID, "Valid session found in Redis, UserID: %d", session.UserID)
	return &session, nil
}

// ClearUserSSOSessions clears all SSO sessions for a user (global logout) - Redis-only
func ClearUserSSOSessions(userID int) {
	if database.RedisClient == nil {
		utils.WarnLog("Redis client not available, cannot clear SSO sessions")
		return
	}

	deletedCount := 0
	ctx := context.Background()

	// Scan all sso_session keys
	iter := database.RedisClient.Scan(ctx, 0, "sso_session:*", 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()

		// Get session data from Redis
		sessionData, err := database.RedisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		// Parse session to check UserID
		var session models.SSOSession
		if err := json.Unmarshal([]byte(sessionData), &session); err != nil {
			continue
		}

		// If this session belongs to the user, delete it
		if session.UserID == userID {
			err := database.RedisClient.Del(ctx, key).Err()
			if err != nil {
				utils.ErrorLog("Failed to delete session from Redis: %v", err)
			} else {
				deletedCount++
				utils.DebugLog("Deleted SSO session from Redis: %s", key)
			}
		}
	}

	if err := iter.Err(); err != nil {
		utils.ErrorLog("Redis scan error during SSO session cleanup: %v", err)
	}

	utils.DebugLog("Cleared %d SSO sessions for user %d", deletedCount, userID)
}

// CleanExpiredSSOTokens cleans up expired SSO sessions from Redis
// Note: Redis TTL handles expiration automatically, but this function
// can be used for explicit cleanup if needed
func CleanExpiredSSOTokens() {
	if database.RedisClient == nil {
		return
	}

	ctx := context.Background()
	deletedCount := 0
	now := time.Now()

	// Scan all sso_session keys
	iter := database.RedisClient.Scan(ctx, 0, "sso_session:*", 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()

		sessionData, err := database.RedisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var session models.SSOSession
		if err := json.Unmarshal([]byte(sessionData), &session); err != nil {
			// Invalid data, delete it
			database.RedisClient.Del(ctx, key)
			deletedCount++
			continue
		}

		if now.After(session.ExpiresAt) {
			database.RedisClient.Del(ctx, key)
			deletedCount++
		}
	}

	if deletedCount > 0 {
		utils.DebugLog("Cleaned up %d expired SSO sessions from Redis", deletedCount)
	}
}
