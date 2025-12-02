package handlers

import (
	"strconv"
	"time"

	"backend/internal/database/api"
	"backend/internal/models"
	"backend/internal/rbac/domain"
	rbacmw "backend/internal/rbac/middleware"
	"backend/internal/rbac/service"
	"backend/pkg/logger"
	"backend/pkg/response"

	"github.com/gofiber/fiber/v2"
)

var log = logger.Default().WithComponent("api-tokens")

// CreateAPIToken creates a new API token for the authenticated user
// Requires: member role or higher (viewers cannot create tokens)
// Note: Access is also controlled by RequireMember middleware in routes
func CreateAPIToken(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)

	var req models.APITokenRequest
	if err := c.BodyParser(&req); err != nil {
		log.WithField("error", err.Error()).Debug("Failed to parse request body")
		return response.BadRequest(c, "Invalid request body")
	}

	// Validate required fields
	if req.Name == "" {
		return response.BadRequest(c, "Token name is required")
	}

	// Create token
	tokenResponse, err := api.APITokens.CreateAPIToken(c.Context(), userID, &req)
	if err != nil {
		log.WithField("error", err.Error()).Error("Failed to create API token")
		return response.InternalServerError(c, "Failed to create token")
	}

	log.WithFields(map[string]interface{}{
		"user_id":      userID,
		"token_prefix": tokenResponse.TokenPrefix,
	}).Info("API token created")

	return response.Created(c, "API token created successfully", tokenResponse)
}

// ListAPITokens returns API tokens
// - Members: Only their own tokens
// - Admins: All tokens for their servers
// Note: Access is also controlled by RequireMember middleware in routes
func ListAPITokens(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)

	tokens, err := api.APITokens.ListAPITokens(c.Context(), userID)
	if err != nil {
		log.WithField("error", err.Error()).Error("Failed to list API tokens")
		return response.InternalServerError(c, "Failed to list tokens")
	}

	return response.SuccessWithMessage(c, "API tokens retrieved successfully", tokens)
}

// DeleteAPIToken deletes an API token
func DeleteAPIToken(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)
	tokenIDStr := c.Params("token_id")

	tokenID, err := strconv.Atoi(tokenIDStr)
	if err != nil {
		return response.BadRequest(c, "Invalid token ID")
	}

	err = api.APITokens.DeleteAPIToken(c.Context(), userID, tokenID)
	if err != nil {
		log.WithFields(map[string]interface{}{
			"user_id":  userID,
			"token_id": tokenID,
			"error":    err.Error(),
		}).Error("Failed to delete API token")
		return response.InternalServerError(c, "Failed to delete token")
	}

	log.WithFields(map[string]interface{}{
		"user_id":  userID,
		"token_id": tokenID,
	}).Info("API token deleted")

	return response.SuccessWithMessage(c, "API token deleted successfully", nil)
}

// GetAppAPIAccess returns API access configuration for a specific app
func GetAppAPIAccess(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return response.BadRequest(c, "App name is required")
	}

	accessConfig, err := api.AppAPIAccess.GetAppAPIAccess(c.Context(), appName)
	if err != nil {
		log.WithFields(map[string]interface{}{
			"app_name": appName,
			"error":    err.Error(),
		}).Debug("Failed to get app API access settings")
		return response.InternalServerError(c, "Failed to get app API access settings")
	}

	return response.SuccessWithMessage(c, "App API access settings retrieved successfully", accessConfig)
}

// SetAppAPIAccess sets API access configuration for a specific app
func SetAppAPIAccess(c *fiber.Ctx) error {
	appName := c.Params("app_name")
	if appName == "" {
		return response.BadRequest(c, "App name is required")
	}

	var req struct {
		APIAccessEnabled   bool `json:"api_access_enabled"`
		RateLimitPerMinute int  `json:"rate_limit_per_minute,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		log.WithField("error", err.Error()).Debug("Failed to parse request body")
		return response.BadRequest(c, "Invalid request body")
	}

	log.WithFields(map[string]interface{}{
		"app_name":   appName,
		"enabled":    req.APIAccessEnabled,
		"rate_limit": req.RateLimitPerMinute,
	}).Debug("SetAppAPIAccess called")

	// Set defaults
	if req.RateLimitPerMinute <= 0 {
		req.RateLimitPerMinute = 60
	}

	// All operations are allowed when API access is enabled
	allowedOperations := []string{"*"}
	err := api.AppAPIAccess.SetAppAPIAccess(c.Context(), appName, req.APIAccessEnabled, allowedOperations, req.RateLimitPerMinute)
	if err != nil {
		log.WithFields(map[string]interface{}{
			"app_name": appName,
			"error":    err.Error(),
		}).Error("Failed to update app API access settings")
		return response.InternalServerError(c, "Failed to update app API access settings")
	}

	// Return the updated config
	accessConfig := &models.AppAPIAccessResponse{
		AppName:            appName,
		APIAccessEnabled:   req.APIAccessEnabled,
		AllowedOperations:  allowedOperations,
		RateLimitPerMinute: req.RateLimitPerMinute,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	log.WithFields(map[string]interface{}{
		"app_name": appName,
		"enabled":  req.APIAccessEnabled,
	}).Info("App API access settings updated")

	return response.SuccessWithMessage(c, "App API access settings updated successfully", accessConfig)
}

// GetAvailableOperations returns simple message that API tokens have full access
func GetAvailableOperations(c *fiber.Ctx) error {
	return response.SuccessWithMessage(c, "API tokens have full access to all operations", fiber.Map{
		"message":    "API tokens provide complete access to all application operations when API access is enabled for an app.",
		"operations": []string{"*"},
		"descriptions": map[string]string{
			"*": "Full access to all operations including deploy, restart, logs, env vars, domains, etc.",
		},
	})
}

// =====================
// Admin Endpoints (Organization-based access via RBAC)
// =====================

// GetTokenAuditLogs returns API token audit logs
// Access control via RBAC:
// - Super admins: See ALL logs
// - Org owners: See all logs in their org
// - Server admins: See logs for their specific apps
// - Members: See only their own token logs
// - Viewers: NO ACCESS
func GetTokenAuditLogs(c *fiber.Ctx) error {
	rbacCtx := rbacmw.GetRBACContext(c)
	if rbacCtx == nil {
		return response.Unauthorized(c, "Authorization context not found")
	}

	// Viewers cannot see audit logs (member+ required)
	if !rbacCtx.GetHighestRole().HasAtLeast(domain.RoleMember) {
		return response.Forbidden(c, "Viewers cannot access audit logs")
	}

	// Parse query params
	eventType := c.Query("event_type", "") // "success", "failed", or empty for all
	limitStr := c.Query("limit", "50")
	offsetStr := c.Query("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 500 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Super admin bypass - can see everything
	if rbacCtx.IsSuperAdmin {
		logs, total, err := api.APITokens.GetAllTokenAuditLogs(c.Context(), eventType, limit, offset)
		if err != nil {
			log.WithField("error", err.Error()).Error("Failed to get audit logs")
			return response.InternalServerError(c, "Failed to get audit logs")
		}
		return response.Success(c, fiber.Map{
			"logs":   logs,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}

	// Set RLS context for non-super-admin users
	rbacService := service.Default()
	if err := rbacService.SetRLSContext(c.Context(), rbacCtx); err != nil {
		log.WithField("error", err.Error()).Error("Failed to set RLS context")
		return response.InternalServerError(c, "Failed to set security context")
	}

	// Get audit logs (RLS filters based on user role and org)
	logs, total, err := api.APITokens.GetTokenAuditLogs(c.Context(), eventType, limit, offset)
	if err != nil {
		log.WithField("error", err.Error()).Error("Failed to get audit logs")
		return response.InternalServerError(c, "Failed to get audit logs")
	}

	return response.Success(c, fiber.Map{
		"logs":   logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetFailedLoginAttempts returns recent failed API token validation attempts grouped by IP
// Access: Super admin OR Org owner/admin only
func GetFailedLoginAttempts(c *fiber.Ctx) error {
	rbacCtx := rbacmw.GetRBACContext(c)
	if rbacCtx == nil {
		return response.Unauthorized(c, "Authorization context not found")
	}

	// Check access: super admin or admin (__all__ + admin)
	if !rbacCtx.IsSuperAdmin && !rbacCtx.Permissions.IsInstanceAdmin() {
		return response.Forbidden(c, "Admin access required")
	}

	hoursStr := c.Query("hours", "24")
	limitStr := c.Query("limit", "20")

	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours <= 0 || hours > 168 { // max 1 week
		hours = 24
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 20
	}

	// Super admin sees all, org owner/admin sees only their org
	targetOrgID := ""
	if !rbacCtx.IsSuperAdmin {
		targetOrgID = rbacCtx.OrganizationID
	}

	attempts, err := api.APITokens.GetRecentFailedAttempts(c.Context(), targetOrgID, hours, limit)
	if err != nil {
		log.WithField("error", err.Error()).Error("Failed to get failed login attempts")
		return response.InternalServerError(c, "Failed to get failed login attempts")
	}

	return response.Success(c, fiber.Map{
		"failed_attempts": attempts,
		"hours":           hours,
	})
}

// GetTokenUsageStats returns usage statistics for a specific token (owner only)
func GetTokenUsageStats(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)
	tokenIDStr := c.Params("token_id")

	tokenID, err := strconv.Atoi(tokenIDStr)
	if err != nil {
		return response.BadRequest(c, "Invalid token ID")
	}

	stats, err := api.APITokens.GetTokenUsageStats(c.Context(), tokenID, userID)
	if err != nil {
		log.WithFields(map[string]interface{}{
			"user_id":  userID,
			"token_id": tokenID,
			"error":    err.Error(),
		}).Debug("Failed to get token usage stats")
		return response.InternalServerError(c, "Failed to get token statistics")
	}

	return response.Success(c, stats)
}
