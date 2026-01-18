// Package service provides the main RBAC service
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"backend/internal/rbac/domain"
	"backend/internal/rbac/repository"
	"backend/pkg/logger"

	"github.com/gofiber/fiber/v2"
)

var log = logger.Default().WithComponent("rbac")

// Service is the main RBAC service
type Service struct {
	repo  *repository.PermissionRepository
	cache *permissionCache
}

// permissionCache provides in-memory caching for permissions
type permissionCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	permissions domain.Permissions
	expiresAt   time.Time
}

// NewService creates a new RBAC service
func NewService() *Service {
	return &Service{
		repo: repository.NewPermissionRepository(),
		cache: &permissionCache{
			entries: make(map[string]*cacheEntry),
			ttl:     2 * time.Minute, // Cache for 2 minutes (reduced for security)
		},
	}
}

// Global service instance
var (
	globalService *Service
	serviceOnce   sync.Once
)

// Default returns the global RBAC service instance
func Default() *Service {
	serviceOnce.Do(func() {
		globalService = NewService()
	})
	return globalService
}

// FromFiber extracts RBAC context from Fiber context
func (s *Service) FromFiber(c *fiber.Ctx) (*domain.RBACContext, error) {
	// Extract user identifiers
	localUserID, _ := c.Locals("user_id").(int)
	citizenAuthUserID, _ := c.Locals("citizenauth_user_id").(string)
	organizationID, _ := c.Locals("organization_id").(string)
	isSuperAdmin, _ := c.Locals("is_super_admin").(bool)

	// Create RBAC context
	ctx := domain.NewRBACContext(localUserID, citizenAuthUserID, organizationID, isSuperAdmin)

	// Load permissions if not super admin
	if !isSuperAdmin && citizenAuthUserID != "" && organizationID != "" {
		permissions, err := s.GetUserPermissions(c.Context(), citizenAuthUserID, organizationID)
		if err != nil {
			log.WithField("error", err.Error()).Warn("Failed to load permissions")
			// Continue with empty permissions
		} else {
			ctx.SetPermissions(permissions)
		}
	}

	return ctx, nil
}

// FromFiberWithCache extracts RBAC context with caching
func FromFiberWithCache(c *fiber.Ctx) (*domain.RBACContext, error) {
	return Default().FromFiber(c)
}

// GetUserPermissions retrieves permissions for a user (with caching)
func (s *Service) GetUserPermissions(ctx context.Context, citizenAuthUserID, organizationID string) (domain.Permissions, error) {
	// Check cache first (using secure hash key)
	cacheKey := generateCacheKey(citizenAuthUserID, organizationID)
	if perms := s.cache.get(cacheKey); perms != nil {
		return perms, nil
	}

	// Load from database
	perms, err := s.repo.GetUserPermissions(ctx, citizenAuthUserID, organizationID)
	if err != nil {
		return nil, err
	}

	// Cache the result
	s.cache.set(cacheKey, perms)

	return perms, nil
}

// InvalidateCache invalidates permissions cache for a user
func (s *Service) InvalidateCache(citizenAuthUserID, organizationID string) {
	cacheKey := generateCacheKey(citizenAuthUserID, organizationID)
	s.cache.delete(cacheKey)
}

// generateCacheKey creates a secure cache key using SHA256 hash
// This prevents cache key collision attacks via delimiter injection
func generateCacheKey(userID, orgID string) string {
	// Use null byte as separator (cannot appear in UUIDs)
	data := userID + "\x00" + orgID
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:16]) // Use first 16 bytes (128 bits) for efficiency
}

// InvalidateAllCache clears the entire permissions cache
func (s *Service) InvalidateAllCache() {
	s.cache.clear()
}

// CheckAppPermission checks if user has required role for an app (with caching)
func (s *Service) CheckAppPermission(ctx context.Context, citizenAuthUserID, organizationID, appID string, requiredRole domain.Role) (bool, error) {
	// Use cached permissions instead of hitting DB every time
	permissions, err := s.GetUserPermissions(ctx, citizenAuthUserID, organizationID)
	if err != nil {
		return false, err
	}

	// Check if user has permission for this specific app
	return permissions.HasAppPermission(appID, requiredRole), nil
}

// IsUserAssignedToInstance checks if user has any permissions in the organization
func (s *Service) IsUserAssignedToInstance(ctx context.Context, citizenAuthUserID, organizationID string) (bool, error) {
	return s.repo.IsUserAssignedToInstance(ctx, citizenAuthUserID, organizationID)
}

// GrantPermission grants or updates permission for a user
func (s *Service) GrantPermission(ctx context.Context, citizenAuthUserID, organizationID, appID string, role domain.Role, grantedBy string) error {
	err := s.repo.GrantPermission(ctx, citizenAuthUserID, organizationID, appID, role, grantedBy)
	if err == nil {
		// Invalidate cache
		s.InvalidateCache(citizenAuthUserID, organizationID)
	}
	return err
}

// RevokePermission revokes a user's permission for an app
func (s *Service) RevokePermission(ctx context.Context, citizenAuthUserID, organizationID, appID string) error {
	err := s.repo.RevokePermission(ctx, citizenAuthUserID, organizationID, appID)
	if err == nil {
		// Invalidate cache
		s.InvalidateCache(citizenAuthUserID, organizationID)
	}
	return err
}

// SetRLSContext sets the session context for Row Level Security
func (s *Service) SetRLSContext(ctx context.Context, rbacCtx *domain.RBACContext) error {
	return s.repo.SetRLSContext(ctx, rbacCtx.LocalUserID, rbacCtx.CitizenAuthUserID, rbacCtx.OrganizationID)
}

// Cache methods
func (c *permissionCache) get(key string) domain.Permissions {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.permissions
}

func (c *permissionCache) set(key string, perms domain.Permissions) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		permissions: perms,
		expiresAt:   time.Now().Add(c.ttl),
	}
}

func (c *permissionCache) delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
}

func (c *permissionCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*cacheEntry)
}

// Cleanup removes expired cache entries (call periodically)
func (c *permissionCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

// StartCacheCleanup starts a background goroutine to clean up expired cache entries
func (s *Service) StartCacheCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.cache.Cleanup()
			case <-ctx.Done():
				return
			}
		}
	}()
}
