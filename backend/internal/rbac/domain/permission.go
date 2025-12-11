// Package domain contains RBAC domain types and constants
package domain

// Permission represents a specific permission from app_permissions table
type Permission struct {
	AppID string // App name or "__all__" for instance-wide
	Role  Role   // admin, member, viewer
}

// Permissions is a list of permissions
type Permissions []Permission

// HasAppPermission checks if permissions include access to a specific app
// Returns true if:
// - User has specific app permission with required role
// - User has __all__ permission with required role
func (p Permissions) HasAppPermission(appID string, requiredRole Role) bool {
	for _, perm := range p {
		// Check specific app permission
		if perm.AppID == appID && perm.Role.HasAtLeast(requiredRole) {
			return true
		}
		// Check instance-wide permission (__all__)
		if perm.AppID == SpecialAppID && perm.Role.HasAtLeast(requiredRole) {
			return true
		}
	}
	return false
}

// HasAnyPermission checks if user has any permission at all
func (p Permissions) HasAnyPermission() bool {
	return len(p) > 0
}

// GetHighestRole returns the highest role across all permissions
func (p Permissions) GetHighestRole() Role {
	highest := RoleViewer
	for _, perm := range p {
		if perm.Role.Level() > highest.Level() {
			highest = perm.Role
		}
	}
	return highest
}

// GetAppsWithRole returns all app IDs where user has at least the specified role
// Excludes "__all__" from the result
func (p Permissions) GetAppsWithRole(minRole Role) []string {
	apps := make([]string, 0)
	for _, perm := range p {
		if perm.Role.HasAtLeast(minRole) && perm.AppID != SpecialAppID {
			apps = append(apps, perm.AppID)
		}
	}
	return apps
}

// IsInstanceAdmin checks if user has __all__ + admin
func (p Permissions) IsInstanceAdmin() bool {
	for _, perm := range p {
		if perm.AppID == SpecialAppID && perm.Role == RoleAdmin {
			return true
		}
	}
	return false
}
