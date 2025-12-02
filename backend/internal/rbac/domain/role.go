// Package domain contains RBAC domain types and constants
package domain

// Role represents a user's role in the app_permissions table
// 3 levels: admin > member > viewer
type Role string

const (
	// RoleViewer - Can only access assigned domains, NO dashboard access
	RoleViewer Role = "viewer"

	// RoleMember - Can deploy to assigned apps
	RoleMember Role = "member"

	// RoleAdmin - Full access (if app_id = "__all__" → instance admin)
	RoleAdmin Role = "admin"
)

// SpecialAppID is used for instance-wide permissions
// User with __all__ + admin = instance admin (same as org owner for this instance)
const SpecialAppID = "__all__"

// RoleHierarchy defines the permission level for each role
// Higher number = more permissions
var RoleHierarchy = map[Role]int{
	RoleViewer: 1,
	RoleMember: 2,
	RoleAdmin:  3,
}

// AllRoles is the list of all valid roles (matching app_permissions CHECK constraint)
var AllRoles = []Role{RoleViewer, RoleMember, RoleAdmin}

// IsValid checks if a role is valid
func (r Role) IsValid() bool {
	_, exists := RoleHierarchy[r]
	return exists
}

// Level returns the hierarchy level of the role
func (r Role) Level() int {
	return RoleHierarchy[r]
}

// HasAtLeast checks if this role has at least the specified role level
func (r Role) HasAtLeast(required Role) bool {
	return r.Level() >= required.Level()
}

// String returns the string representation of the role
func (r Role) String() string {
	return string(r)
}

// ParseRole converts a string to Role
func ParseRole(s string) Role {
	role := Role(s)
	if role.IsValid() {
		return role
	}
	return RoleViewer // Default to viewer for invalid roles
}
