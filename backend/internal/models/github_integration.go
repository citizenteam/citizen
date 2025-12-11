package models

import (
	"time"
)

// GitHubConfig represents encrypted GitHub OAuth configuration
type GitHubConfig struct {
	ID            uint      `json:"id" gorm:"primaryKey"`
	ClientID      string    `json:"-" gorm:"not null"`              // Encrypted GitHub Client ID
	ClientSecret  string    `json:"-" gorm:"not null"`              // Encrypted GitHub Client Secret
	WebhookSecret string    `json:"-" gorm:"not null"`              // Encrypted Webhook Secret
	RedirectURI   string    `json:"redirect_uri" gorm:"not null"`   // Plain text redirect URI
	IsActive      bool      `json:"is_active" gorm:"default:true"`  // Active configuration
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// GitHubRepository represents a connected GitHub repository
type GitHubRepository struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	UserID      uint      `json:"user_id" gorm:"not null;index"`
	AppName     string    `json:"app_name" gorm:"unique;not null"` // Citizen app name
	GitHubID    int64     `json:"github_id" gorm:"not null"`       // GitHub repo ID
	FullName    string    `json:"full_name" gorm:"not null"`       // owner/repo-name
	Name        string    `json:"name" gorm:"not null"`            // repo-name
	Owner       string    `json:"owner" gorm:"not null"`           // owner
	CloneURL    string    `json:"clone_url" gorm:"not null"`       // Git clone URL
	HTMLURL     string    `json:"html_url" gorm:"not null"`        // GitHub web URL
	Private     bool      `json:"private" gorm:"default:false"`
	DefaultBranch string  `json:"default_branch" gorm:"default:main"`
	
	// Auto Deploy Settings
	AutoDeployEnabled bool   `json:"auto_deploy_enabled" gorm:"default:false"`
	DeployBranch      string `json:"deploy_branch" gorm:"default:main"`
	
	// Webhook Info
	WebhookID     *int64  `json:"webhook_id,omitempty"`     // GitHub webhook ID
	WebhookSecret *string `json:"-"`                        // Webhook secret (hidden)
	WebhookActive bool    `json:"webhook_active" gorm:"default:false"`
	
	// Timestamps
	ConnectedAt time.Time  `json:"connected_at"`
	LastDeploy  *time.Time `json:"last_deploy,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" gorm:"index"`
	
	// Relations
	User User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// GitHubWebhookEvent represents a GitHub webhook event
type GitHubWebhookEvent struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	RepositoryID uint      `json:"repository_id" gorm:"not null;index"`
	EventType    string    `json:"event_type" gorm:"not null"` // push, pull_request, etc.
	Action       string    `json:"action,omitempty"`           // opened, closed, etc.
	Ref          string    `json:"ref,omitempty"`              // refs/heads/main
	Before       string    `json:"before,omitempty"`           // commit hash before
	After        string    `json:"after,omitempty"`            // commit hash after
	
	// Payload Info
	PayloadSize  int    `json:"payload_size"`
	PayloadHash  string `json:"payload_hash"`  // SHA256 of payload
	GitHubID     string `json:"github_id"`     // GitHub delivery ID
	
	// Processing Status
	Processed     bool       `json:"processed" gorm:"default:false"`
	ProcessedAt   *time.Time `json:"processed_at,omitempty"`
	DeployTriggered bool     `json:"deploy_triggered" gorm:"default:false"`
	DeploySuccess   *bool    `json:"deploy_success,omitempty"`
	ErrorMessage    *string  `json:"error_message,omitempty"`
	
	// Timestamps
	ReceivedAt time.Time `json:"received_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	
	// Relations
	Repository GitHubRepository `json:"repository,omitempty" gorm:"foreignKey:RepositoryID"`
}

// GitHubDeploymentLog represents auto deployment logs
type GitHubDeploymentLog struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	RepositoryID uint      `json:"repository_id" gorm:"not null;index"`
	EventID      *uint     `json:"event_id,omitempty"`     // Link to webhook event
	AppName      string    `json:"app_name" gorm:"not null"`
	
	// Git Info
	CommitHash   string `json:"commit_hash" gorm:"not null"`
	CommitMsg    string `json:"commit_message"`
	Branch       string `json:"branch" gorm:"not null"`
	AuthorName   string `json:"author_name"`
	AuthorEmail  string `json:"author_email"`
	
	// Deploy Info
	TriggerType  string `json:"trigger_type"` // webhook, manual, schedule
	Status       string `json:"status"`       // pending, running, success, failed
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Duration     *int       `json:"duration,omitempty"` // seconds
	
	// Logs
	BuildOutput  *string `json:"build_output,omitempty"`
	ErrorOutput  *string `json:"error_output,omitempty"`
	
	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	
	// Relations
	Repository GitHubRepository     `json:"repository,omitempty" gorm:"foreignKey:RepositoryID"`
	Event      *GitHubWebhookEvent  `json:"event,omitempty" gorm:"foreignKey:EventID"`
} 