package models

// SessionWebhookPayload represents session webhook payload from CitizenAuth
type SessionWebhookPayload struct {
	Event          string `json:"event"`
	UserID         string `json:"user_id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	OrganizationID string `json:"organization_id"`
	SessionID      string `json:"session_id"`
	Token          string `json:"token"`
	Timestamp      int64  `json:"timestamp"`
}

// PermissionWebhookPayload represents permission webhook payload from CitizenAuth
type PermissionWebhookPayload struct {
	Event          string `json:"event"` // permission.granted, permission.revoked
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	AppID          string `json:"app_id"`
	Role           string `json:"role"`
	GrantedBy      string `json:"granted_by"`
	Timestamp      int64  `json:"timestamp"`
}

// InstanceLifecycleWebhookPayload represents instance lifecycle webhook payload from CitizenAuth
type InstanceLifecycleWebhookPayload struct {
	Event         string `json:"event"`
	InstanceID    string `json:"instance_id"`
	Reason        string `json:"reason"`
	AppName       string `json:"app_name"`
	Domain        string `json:"domain"`
	ChallengeURL  string `json:"challenge_url"`
	ChallengeBody string `json:"challenge_body"`
}
