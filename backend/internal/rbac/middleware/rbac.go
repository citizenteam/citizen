// Package middleware provides Fiber middleware for RBAC
package middleware

import (
	"backend/internal/rbac/domain"
	"backend/internal/rbac/service"
	"backend/pkg/logger"
	"backend/pkg/response"

	"github.com/gofiber/fiber/v2"
)

var log = logger.Default().WithComponent("rbac-middleware")

// rbacContextKey is the key used to store RBAC context in Fiber locals
const rbacContextKey = "rbac_context"

// LoadRBACContext middleware loads RBAC context for the request
func LoadRBACContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rbacService := service.Default()
		rbacCtx, err := rbacService.FromFiber(c)
		if err != nil {
			log.WithField("error", err.Error()).Error("Failed to load RBAC context")
			return response.InternalServerError(c, "Failed to load authorization context")
		}

		// Store in Fiber context
		c.Locals(rbacContextKey, rbacCtx)

		return c.Next()
	}
}

// GetRBACContext retrieves RBAC context from Fiber context
func GetRBACContext(c *fiber.Ctx) *domain.RBACContext {
	ctx, ok := c.Locals(rbacContextKey).(*domain.RBACContext)
	if !ok {
		return nil
	}
	return ctx
}

// RequireDashboardAccess ensures user can access the dashboard (member+)
// Viewers cannot access dashboard - they only get domain-based access
func RequireDashboardAccess() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rbacCtx := GetRBACContext(c)
		if rbacCtx == nil {
			return response.Unauthorized(c, "Authorization context not found")
		}

		if rbacCtx.IsSuperAdmin {
			return c.Next()
		}

		if !rbacCtx.GetHighestRole().HasAtLeast(domain.RoleMember) {
			return response.Forbidden(c, "Viewers cannot access the dashboard")
		}

		return c.Next()
	}
}

// RequireAdmin ensures user has admin role (anywhere: __all__ or specific app)
// Use this for: creating apps, managing server settings, viewing all logs
func RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rbacCtx := GetRBACContext(c)
		if rbacCtx == nil {
			return response.Unauthorized(c, "Authorization context not found")
		}

		if rbacCtx.IsSuperAdmin {
			return c.Next()
		}

		if !rbacCtx.GetHighestRole().HasAtLeast(domain.RoleAdmin) {
			return response.Forbidden(c, "Admin access required")
		}

		return c.Next()
	}
}

// RequireMember ensures user has at least member role (anywhere)
// Use this for: creating tokens, general member actions
func RequireMember() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rbacCtx := GetRBACContext(c)
		if rbacCtx == nil {
			return response.Unauthorized(c, "Authorization context not found")
		}

		if rbacCtx.IsSuperAdmin {
			return c.Next()
		}

		if !rbacCtx.GetHighestRole().HasAtLeast(domain.RoleMember) {
			return response.Forbidden(c, "Member access required")
		}

		return c.Next()
	}
}

// RequireAppAdmin ensures user has admin role for the specific app (from :app_name param)
// Checks: __all__ + admin OR specific app + admin
func RequireAppAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rbacCtx := GetRBACContext(c)
		if rbacCtx == nil {
			return response.Unauthorized(c, "Authorization context not found")
		}

		if rbacCtx.IsSuperAdmin {
			return c.Next()
		}

		appID := c.Params("app_name")
		if appID == "" {
			return response.BadRequest(c, "App name required")
		}

		if !rbacCtx.Permissions.HasAppPermission(appID, domain.RoleAdmin) {
			return response.Forbidden(c, "Admin access required for this app")
		}

		return c.Next()
	}
}

// RequireAppMember ensures user has at least member role for the specific app
// Checks: __all__ + member OR specific app + member
// Use this for: deploying to app
func RequireAppMember() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rbacCtx := GetRBACContext(c)
		if rbacCtx == nil {
			return response.Unauthorized(c, "Authorization context not found")
		}

		if rbacCtx.IsSuperAdmin {
			return c.Next()
		}

		appID := c.Params("app_name")
		if appID == "" {
			return response.BadRequest(c, "App name required")
		}

		if !rbacCtx.Permissions.HasAppPermission(appID, domain.RoleMember) {
			return response.Forbidden(c, "Member access required for this app")
		}

		return c.Next()
	}
}

// RequireAppViewer ensures user has at least viewer role for the specific app
// Checks: __all__ + viewer OR specific app + viewer
// Use this for: viewing app details, logs
func RequireAppViewer() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rbacCtx := GetRBACContext(c)
		if rbacCtx == nil {
			return response.Unauthorized(c, "Authorization context not found")
		}

		if rbacCtx.IsSuperAdmin {
			return c.Next()
		}

		appID := c.Params("app_name")
		if appID == "" {
			return response.BadRequest(c, "App name required")
		}

		if !rbacCtx.Permissions.HasAppPermission(appID, domain.RoleViewer) {
			return response.Forbidden(c, "Access denied for this app")
		}

		return c.Next()
	}
}

// SetRLSContext middleware sets RLS context for database queries
func SetRLSContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rbacCtx := GetRBACContext(c)
		if rbacCtx == nil {
			return c.Next() // No RBAC context, skip RLS
		}

		rbacService := service.Default()
		if err := rbacService.SetRLSContext(c.Context(), rbacCtx); err != nil {
			log.WithField("error", err.Error()).Warn("Failed to set RLS context")
		}

		return c.Next()
	}
}
